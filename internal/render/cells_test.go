package render

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgeIsShortAndCoarse(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	for ago, want := range map[time.Duration]string{
		10 * time.Second: "now",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	} {
		if got := Age(start, start.Add(-ago)); got != want {
			t.Errorf("%s ago: got %q, want %q", ago, got, want)
		}
	}
	if got := Age(start, time.Time{}); got != "-" {
		t.Errorf("no start time: got %q", got)
	}
}

func TestCells(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	three := 3
	for _, tc := range []struct{ got, want string }{
		{Ago(start, start), "now"},
		{Ago(start, start.Add(-50*time.Hour)), "2d ago"},
		{Ago(start, time.Time{}), "unknown"},
		{BranchCell(""), "-"},
		{BranchCell("feat/ledger-rounding-for-every-currency"), "feat/…for-every-currency"},
		{CountCell(nil), "-"},
		{CountCell(new(int)), "-"},
		{CountCell(&three), "3"},
		{FirstLine("one\ntwo", 10), "one …"},
		{FirstLine("a longer line", 5), "a lo…"},
		{NameCell("api\x1b[31m"), "api[31m"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
	if got := DirRoom(0, 30); got != MaxDir {
		t.Errorf("off a terminal, a directory gets %d columns, want %d", got, MaxDir)
	}
	if got := DirRoom(50, 40); got != MinDir {
		t.Errorf("on a narrow terminal, a directory gets %d columns, want %d", got, MinDir)
	}
}

func TestTokensReadAsAgentViewDoes(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0", 30: "30", 999: "999", 1000: "1.0K", 25_080: "25K", 171_170: "171K",
		999_600: "1.0M", 1_400_000: "1.4M", 3_956_924: "4.0M", 61_000_000: "61M", 2_500_000_000: "2.5B",
	} {
		if got := Tokens(&n); got != want {
			t.Errorf("Tokens(%d) = %q, want %q", n, got, want)
		}
	}
	if got := Tokens(nil); got != "-" {
		t.Errorf("unknown tokens show %q", got)
	}
}

func TestHomeShort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sep := string(filepath.Separator)
	for path, want := range map[string]string{
		home:                                  "~",
		filepath.Join(home, "work", "repo"):   "~" + sep + filepath.Join("work", "repo"),
		home + "-other":                       home + "-other",
		filepath.Join(os.TempDir(), "x", "y"): filepath.Join(os.TempDir(), "x", "y"),
	} {
		if got := HomeShort(path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}
