package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/config"
)

func (app *App) configEditCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Open config.yaml in $VISUAL or $EDITOR, starting it if it is missing",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if err := app.configPathErr(); err != nil {
				return err
			}
			if err := config.Create(app.ConfigPath); err != nil {
				return err
			}
			if err := app.Edit(app.ConfigPath); err != nil {
				return err
			}
			if _, err := config.Load(app.ConfigPath); err != nil {
				return fmt.Errorf("the file was saved, but claudes cannot use it: %w", err)
			}
			return nil
		},
	}
}

// editFile opens path in $VISUAL or $EDITOR, which may carry arguments such as
// "code --wait", and waits for it to close.
func editFile(path string) error {
	editor := strings.Fields(os.Getenv("VISUAL"))
	if len(editor) == 0 {
		editor = strings.Fields(os.Getenv("EDITOR"))
	}
	if len(editor) == 0 {
		editor = []string{defaultEditor}
	}
	if _, err := exec.LookPath(editor[0]); err != nil {
		return errors.New("no editor found: set VISUAL or EDITOR, or edit " + path + " by hand")
	}
	cmd := exec.Command(editor[0], append(editor[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
