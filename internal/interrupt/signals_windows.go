//go:build windows

package interrupt

import "os"

// stopSignals end a run as Ctrl-C does: on Windows, Ctrl-C and Ctrl-Break.
var stopSignals = []os.Signal{os.Interrupt}
