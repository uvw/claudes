package style

import (
	"bytes"
	"fmt"
	"io"
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
	for _, tc := range []struct {
		name string
		env  []string
		want bool
	}{
		{"not a terminal", []string{"TERM=xterm-256color"}, false},
		{"a terminal", terminal, true},
		{"a dumb terminal", []string{"TERM=dumb"}, false},
		{"NO_COLOR on a terminal", append([]string{"NO_COLOR=1"}, terminal...), false},
		{"NO_COLOR set to anything", append([]string{"NO_COLOR=yes"}, terminal...), false},
		{"NO_COLOR over CLICOLOR=1", []string{"NO_COLOR=1", "CLICOLOR=1"}, false},
		{"CLICOLOR=0 on a terminal", append([]string{"CLICOLOR=0"}, terminal...), false},
		{"CLICOLOR=1 when not a terminal", []string{"CLICOLOR=1"}, true},
		{"the last value wins", []string{"CLICOLOR=0", "CLICOLOR=1"}, true},
		{"an empty NO_COLOR", []string{"NO_COLOR=", "CLICOLOR=1"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name != "not a terminal" {
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
