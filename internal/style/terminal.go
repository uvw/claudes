package style

import (
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// TerminalWidth is how many columns the terminal w writes to has, or 0 when w
// is not a terminal.
func TerminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !isTerminal(w) {
		return 0
	}
	width, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0
	}
	return width
}
