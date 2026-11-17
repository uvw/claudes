//go:build windows

package shell

// Windows Terminal opens PowerShell by default, and cmd cannot run a command
// that changes directories and quotes safely in one line.
var (
	resumeCommand = powerShellResume
	removeCommand = powerShellRemove
	arg           = powerShellArg
)
