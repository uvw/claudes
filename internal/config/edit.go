package config

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Edit opens path in $VISUAL or $EDITOR, which may carry arguments such as
// "code --wait", and waits for it to close.
func Edit(path string) error {
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
