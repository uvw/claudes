package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// cdCommand runs only when the shell integration is not loaded: the shell
// function handles claudes cd itself.
func (app *App) cdCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "cd TARGET",
		Short:             "Change to a session's directory (needs claudes init)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: app.completePast(true),
		RunE: func(*cobra.Command, []string) error {
			return errors.New(`claudes cd needs the shell integration, since a program cannot change its shell's directory. Add one line to your shell's profile:
  bash:  eval "$(claudes init bash)"
  zsh:   eval "$(claudes init zsh)"
  fish:  claudes init fish | source
  pwsh:  claudes init pwsh | Out-String | Invoke-Expression`)
		},
	}
}
