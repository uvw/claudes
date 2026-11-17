//go:build windows

package claude

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// platformEnv stops cmd.exe from looking for programs in the current directory,
// the session's. npm's claude.cmd runs a bare node, so a node.exe or
// node.cmd in a cloned repository would otherwise run in its place.
var platformEnv = []string{"NoDefaultCurrentDirectoryInExePath=1"}

// isolate starts cmd in its own process group, so the Ctrl-C a console sends
// to claudes does not also kill the claude call claudes is finishing.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}
