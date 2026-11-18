package style

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"text/tabwriter"
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

		{"NO_COLOR over CLICOLOR_FORCE", pipe, []string{"NO_COLOR=1", "CLICOLOR_FORCE=1"}, false},
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

func TestTableLinesUpAsTabwriterDoes(t *testing.T) {
	rows := [][]string{
		{"RESULT", "CHECK", "DETAIL"},
		{"ok", "claude", "2.1.330 at /bin/claude"},
		{"fail", "agents --json", "first problem"},
		{"", "", "second problem"},
		{"warn", "", ""},
	}
	var want bytes.Buffer
	tw := tabwriter.NewWriter(&want, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	tw.Flush()

	var plain bytes.Buffer
	if err := Table(&plain, rows); err != nil {
		t.Fatal(err)
	}
	if plain.String() != want.String() {
		t.Fatalf("got:\n%s\nwant:\n%s", plain.String(), want.String())
	}

	colored := make([][]string, len(rows))
	for i, r := range rows {
		colored[i] = []string{On.Red(r[0]), r[1], On.Faint(r[2])}
	}
	var out bytes.Buffer
	if err := Table(&out, colored); err != nil {
		t.Fatal(err)
	}
	if !sgr.MatchString(out.String()) {
		t.Fatalf("no colors:\n%q", out.String())
	}
	if got := sgr.ReplaceAllString(out.String(), ""); got != want.String() {
		t.Fatalf("colored table without its colors:\n%s\nwant:\n%s", got, want.String())
	}
}

func TestSizeReadsAsDuDoes(t *testing.T) {
	for n, want := range map[int64]string{
		0:             "0B",
		512:           "512B",
		1024:          "1.0K",
		12 * 1024:     "12K",
		3_500_000:     "3.3M",
		1_300_000_000: "1.2G",
		5 << 40:       "5.0T",
	} {
		if got := Size(n); got != want {
			t.Errorf("Size(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestIndentIsWhereTheLastCellStarts(t *testing.T) {
	rows := [][]string{{"ID", "NAME", "DOING"}, {"abf00936", On.Green("a name"), "tests"}}
	var b strings.Builder
	if err := Table(&b, rows); err != nil {
		t.Fatal(err)
	}
	line := strings.Split(b.String(), "\n")[1]
	if got, want := Indent(rows), Width(line)-len("tests"); got != want {
		t.Errorf("Indent is %d, the last cell starts at %d in %q", got, want, line)
	}
	if Indent(nil) != 0 {
		t.Error("an empty table has an indent")
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
