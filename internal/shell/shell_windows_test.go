//go:build windows

package shell

// posixShells is empty: Windows prints PowerShell commands, and the sh that
// Git installs rewrites arguments that look like paths.
func posixShells() [][]string { return nil }
