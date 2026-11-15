package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// posixInit wraps claudes in a shell function, because only the shell itself
// can change its folder. Every other command goes to the claudes program.
const posixInit = `# claudes shell integration: claudes cd NAME changes to a session's folder.
claudes() {
  if [ "$1" = cd ]; then
    shift
    local dir
    dir="$(command claudes session where --one -o text -- "$@")" && cd -- "$dir"
  else
    command claudes "$@"
  fi
}
`

const pwshInit = `# claudes shell integration: claudes cd NAME changes to a session's folder.
function claudes {
  $exe = Get-Command claudes -CommandType Application -ErrorAction Stop | Select-Object -First 1
  if ($args.Count -gt 0 -and $args[0] -eq 'cd') {
    $rest = @($args | Select-Object -Skip 1)
    # claudes prints UTF-8, which PowerShell reads in the console's code page.
    $encoding = [Console]::OutputEncoding
    [Console]::OutputEncoding = [Text.Encoding]::UTF8
    try { $dir = & $exe session where --one -o text '--' @rest }
    finally { [Console]::OutputEncoding = $encoding }
    if ($LASTEXITCODE -eq 0 -and $dir) { Set-Location -LiteralPath $dir }
  } else {
    & $exe @args
  }
}
`

var shellInits = map[string]string{"bash": posixInit, "zsh": posixInit, "pwsh": pwshInit, "powershell": pwshInit}

func (app *App) initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init bash|zsh|pwsh",
		Short: "Print the shell integration that makes claudes cd work",
		Long: `Print a shell function that makes claudes cd NAME change your shell's folder
to a session's folder. Load it from your shell's profile:

  bash:  eval "$(claudes init bash)"
  zsh:   eval "$(claudes init zsh)"
  pwsh:  claudes init pwsh | Out-String | Invoke-Expression

Every other command runs claudes as before.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "pwsh"},
		RunE: func(_ *cobra.Command, args []string) error {
			script, ok := shellInits[args[0]]
			if !ok {
				return fmt.Errorf("no integration for %q: use bash, zsh, or pwsh", args[0])
			}
			fmt.Fprint(app.Out, script)
			return nil
		},
	}
}

// cdCommand runs only when the shell integration is not loaded: the shell
// function handles claudes cd itself.
func (app *App) cdCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "cd TARGET",
		Short:             "Change to a session's folder (needs claudes init)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completePast(true),
		RunE: func(*cobra.Command, []string) error {
			return errors.New(`claudes cd needs the shell integration, since a program cannot change its shell's folder. Add one line to your shell's profile:
  bash:  eval "$(claudes init bash)"
  zsh:   eval "$(claudes init zsh)"
  pwsh:  claudes init pwsh | Out-String | Invoke-Expression`)
		},
	}
}
