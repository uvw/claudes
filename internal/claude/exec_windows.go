//go:build windows

package claude

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
)

// Exec runs the named binary in this terminal and exits with its exit code.
// Windows cannot replace a running process, so claudes waits for the child
// instead. It returns only when the binary cannot be started.
func Exec(binary string, args ...string) error {
	path, err := exec.LookPath(binary)
	if err != nil {
		return err
	}
	if err := checkArgs(path, args); err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = childEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl-C and Ctrl-Break reach every process on the console; the child
	// decides what they mean, and claudes waits for it to exit. The handler
	// must be a channel: with signal.Ignore, Go declines the console event and
	// Windows ends claudes at once, orphaning the child.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	if err := cmd.Start(); err != nil {
		return err
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
