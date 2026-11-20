//go:build unix

package claude

import (
	"os/exec"
	"syscall"
)

// platformEnv adds nothing on Unix.
var platformEnv []string

// isolate starts cmd in its own process group, so the Ctrl-C a terminal sends
// to claudes does not also kill the claude call claudes is finishing.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
