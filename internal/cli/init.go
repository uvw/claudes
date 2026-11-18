package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// posixInit wraps claudes in a shell function, because only the shell itself
// can change its directory. Every other command goes to the claudes program.
const posixInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
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

const pwshInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
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

// fishInit mirrors posixInit. Fish sets $status after a command substitution,
// and string collect keeps a directory with a trailing newline or spaces whole.
const fishInit = `# claudes shell integration: claudes cd NAME changes to a session's directory.
function claudes --wraps claudes --description 'claudes, with claudes cd NAME'
    if test (count $argv) -gt 0; and test "$argv[1]" = cd
        set -l dir (command claudes session where --one -o text -- $argv[2..-1] | string collect)
        and cd -- $dir
    else
        command claudes $argv
    end
end
`

var shellInits = map[string]string{"bash": posixInit, "zsh": posixInit, "fish": fishInit, "pwsh": pwshInit, "powershell": pwshInit}

func (app *App) initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init bash|zsh|fish|pwsh",
		Short: "Print the shell integration that makes claudes cd work",
		Long: `Print a shell function that makes claudes cd NAME change your shell's directory
to a session's directory. Load it from your shell's profile:

  bash:  eval "$(claudes init bash)"
  zsh:   eval "$(claudes init zsh)"
  fish:  claudes init fish | source
  pwsh:  claudes init pwsh | Out-String | Invoke-Expression

Every other command runs claudes as before.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "pwsh"},
		RunE: func(_ *cobra.Command, args []string) error {
			script, ok := shellInits[args[0]]
			if !ok {
				return fmt.Errorf("no integration for %q: use bash, zsh, fish, or pwsh", args[0])
			}
			fmt.Fprint(app.Out, script)
			return nil
		},
	}
}
