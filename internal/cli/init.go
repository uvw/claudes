package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
)

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
			script, ok := shell.Init(args[0])
			if !ok {
				return fmt.Errorf("no integration for %q: use bash, zsh, fish, or pwsh", args[0])
			}
			fmt.Fprint(app.Out, script)
			return nil
		},
	}
}
