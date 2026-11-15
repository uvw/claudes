package style

import (
	"bytes"
	"testing"
)

// Detect's doc names only CLICOLOR=1 as the way to color a pipe.
func TestOnlyCLICOLORColorsAPipe(t *testing.T) {
	for _, env := range [][]string{
		{"CLICOLOR_FORCE=1"},
		{"TTY_FORCE=1", "TERM=xterm-256color"},
		{"TTY_FORCE=1", "WT_SESSION=abc"},
	} {
		if Detect(&bytes.Buffer{}, env).Colors() {
			t.Errorf("%v colors a pipe", env)
		}
	}
}

// lipgloss Render pads every line of a multi-line string to the widest one and
// turns tabs into spaces, so a colored error differs from a plain one.
func TestRedKeepsTheTextAsIs(t *testing.T) {
	text := "short\na much longer line\tend"
	got := On.Red(text)
	plain := stripANSI(got)
	if plain != text {
		t.Fatalf("Red changed the text: %q", plain)
	}
}

func stripANSI(s string) string {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i++; i < len(s) && !(s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z'); i++ {
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
