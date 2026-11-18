//go:build unix

package cli

import (
	"os"
	"syscall"
)

// stopSignals end a run as Ctrl-C does. A closed terminal sends SIGHUP, and
// a tool that gives up on claudes, such as a timeout, sends SIGTERM.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
