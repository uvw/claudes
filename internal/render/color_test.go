package render

import (
	"bytes"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

// terminal is the environment of a terminal that shows colors. The tests
// that use it also take every writer for a terminal.
var terminal = []string{"TERM=xterm-256color"}

// asTerminal makes Detect take every writer for a terminal while a test runs.
func asTerminal(t *testing.T) {
	saved := isTerminal
	isTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { isTerminal = saved })
}

func TestDetect(t *testing.T) {
	const pipe, tty = false, true
	for _, tc := range []struct {
		name string
		tty  bool
		env  []string
		want bool
	}{
		{"a pipe", pipe, terminal, false},
		{"a terminal", tty, terminal, true},
		{"a dumb terminal", tty, []string{"TERM=dumb"}, false},

		{"NO_COLOR on a terminal", tty, append([]string{"NO_COLOR=1"}, terminal...), false},
		{"NO_COLOR set to anything", tty, append([]string{"NO_COLOR=yes"}, terminal...), false},
		{"NO_COLOR=0", tty, append([]string{"NO_COLOR=0"}, terminal...), false},
		{"an empty NO_COLOR", tty, append([]string{"NO_COLOR="}, terminal...), true},

		{"CLICOLOR=1 on a terminal", tty, append([]string{"CLICOLOR=1"}, terminal...), true},
		{"CLICOLOR=1 on a pipe", pipe, append([]string{"CLICOLOR=1"}, terminal...), false},
		{"CLICOLOR=1 on a dumb terminal", tty, []string{"CLICOLOR=1", "TERM=dumb"}, false},
		{"CLICOLOR=0 on a terminal", tty, append([]string{"CLICOLOR=0"}, terminal...), false},
		{"an empty CLICOLOR on a terminal", tty, append([]string{"CLICOLOR="}, terminal...), true},

		{"CLICOLOR_FORCE=1 on a pipe", pipe, []string{"CLICOLOR_FORCE=1"}, true},
		{"CLICOLOR_FORCE set to anything", pipe, []string{"CLICOLOR_FORCE=yes"}, true},
		{"CLICOLOR_FORCE on a dumb terminal", tty, []string{"CLICOLOR_FORCE=1", "TERM=dumb"}, true},
		{"CLICOLOR_FORCE=0 on a pipe", pipe, []string{"CLICOLOR_FORCE=0"}, false},
		{"CLICOLOR_FORCE=0 on a terminal", tty, append([]string{"CLICOLOR_FORCE=0"}, terminal...), true},
		{"an empty CLICOLOR_FORCE on a pipe", pipe, []string{"CLICOLOR_FORCE="}, false},

		{"FORCE_COLOR=1 on a pipe", pipe, []string{"FORCE_COLOR=1"}, true},
		{"FORCE_COLOR set to anything", pipe, []string{"FORCE_COLOR=true"}, true},
		{"FORCE_COLOR on a dumb terminal", tty, []string{"FORCE_COLOR=1", "TERM=dumb"}, true},
		{"FORCE_COLOR=0 on a pipe", pipe, []string{"FORCE_COLOR=0"}, false},
		{"FORCE_COLOR=0 on a terminal", tty, append([]string{"FORCE_COLOR=0"}, terminal...), true},
		{"an empty FORCE_COLOR on a pipe", pipe, []string{"FORCE_COLOR="}, false},
		{"FORCE_COLOR=0 leaves CLICOLOR_FORCE", pipe, []string{"FORCE_COLOR=0", "CLICOLOR_FORCE=1"}, true},
		{"CLICOLOR_FORCE=0 leaves FORCE_COLOR", pipe, []string{"CLICOLOR_FORCE=0", "FORCE_COLOR=1"}, true},

		{"NO_COLOR over CLICOLOR_FORCE", pipe, []string{"NO_COLOR=1", "CLICOLOR_FORCE=1"}, false},
		{"NO_COLOR over FORCE_COLOR", pipe, []string{"NO_COLOR=1", "FORCE_COLOR=1"}, false},
		{"FORCE_COLOR over CLICOLOR=0", pipe, []string{"CLICOLOR=0", "FORCE_COLOR=1"}, true},
		{"NO_COLOR over CLICOLOR=1", tty, append([]string{"NO_COLOR=1", "CLICOLOR=1"}, terminal...), false},
		{"CLICOLOR_FORCE over CLICOLOR=0", pipe, []string{"CLICOLOR=0", "CLICOLOR_FORCE=1"}, true},
		{"CLICOLOR_FORCE=0 leaves CLICOLOR=0", tty, append([]string{"CLICOLOR=0", "CLICOLOR_FORCE=0"}, terminal...), false},
		{"an empty NO_COLOR leaves CLICOLOR_FORCE", pipe, []string{"NO_COLOR=", "CLICOLOR_FORCE=1"}, true},

		{"the last CLICOLOR wins", tty, append([]string{"CLICOLOR=0", "CLICOLOR=1"}, terminal...), true},
		{"the last CLICOLOR_FORCE wins", pipe, []string{"CLICOLOR_FORCE=1", "CLICOLOR_FORCE=0"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tty {
				asTerminal(t)
			}
			if got := Detect(&bytes.Buffer{}, tc.env).Colors(); got != tc.want {
				t.Fatalf("colors %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAPaletteThatIsOffLeavesTextPlain(t *testing.T) {
	var p Palette
	for _, paint := range []func(string) string{p.Red, p.Green, p.Yellow, p.Blue, p.Magenta, p.Cyan, p.Faint} {
		if got := paint("text"); got != "text" {
			t.Fatalf("got %q", got)
		}
	}
}

func TestOnColorsEachWordAndNoSpace(t *testing.T) {
	for _, paint := range []func(string) string{On.Red, On.Green, On.Yellow, On.Blue, On.Magenta, On.Cyan, On.Faint} {
		got := paint("text")
		if got == "text" || sgr.ReplaceAllString(got, "") != "text" {
			t.Fatalf("got %q", got)
		}
	}
	if got := On.Red(""); got != "" {
		t.Fatalf("an empty string became %q", got)
	}
	if got := Pad("ok", 5, On.Green); !strings.HasSuffix(got, "m   ") || Width(got) != 5 {
		t.Fatalf("Pad: %q", got)
	}
}

func TestTerminalWidthOfWhatIsNoTerminal(t *testing.T) {
	if w := TerminalWidth(&strings.Builder{}); w != 0 {
		t.Errorf("a buffer has width %d", w)
	}
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if w := TerminalWidth(f); w != 0 {
		t.Errorf("a file has width %d", w)
	}
	asTerminal(t)
	if w := TerminalWidth(f); w != 0 {
		t.Errorf("a file taken for a terminal has width %d", w)
	}
}

// Detect's doc names only CLICOLOR_FORCE as the way to color a pipe. Many
// macOS users export CLICOLOR=1 for ls, and colorprofile honors TTY_FORCE.
func TestOnlyCLICOLORFORCEColorsAPipe(t *testing.T) {
	for _, env := range [][]string{
		{"CLICOLOR=1"},
		{"CLICOLOR=1", "TERM=xterm-256color"},
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
			// The letter test reads better whole than with De Morgan's law applied.
			for i++; i < len(s) && !(s[i] >= 'A' && s[i] <= 'Z' || s[i] >= 'a' && s[i] <= 'z'); i++ { //nolint:staticcheck
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
