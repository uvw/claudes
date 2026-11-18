package style

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	deep     = "~/code/acme/monorepo/services/billing/internal/ledger/testdata/v2"
	deepAbs  = "/srv/build/workspace/.cache/go-build/7f/objects/main"
	longLast = "~/x/a-directory-name-far-too-long-to-fit-in-forty-columns"
)

func TestShortDir(t *testing.T) {
	for _, tc := range []struct {
		path  string
		limit int
		how   string
		want  string
	}{
		{deep, 40, DirsMiddle, "~/code/…/internal/ledger/testdata/v2"},
		{deep, 30, DirsMiddle, "~/code/…/ledger/testdata/v2"},
		{deep, 20, DirsMiddle, "~/code/…/testdata/v2"},
		{deepAbs, 40, DirsMiddle, "/srv/…/.cache/go-build/7f/objects/main"},
		{deepAbs, 20, DirsMiddle, "/srv/…/objects/main"},
		{longLast, 40, DirsMiddle, "~/…/a-directory-name-f…-in-forty-columns"},
		{"~/a-directory-name-far-too-long-to-fit-in-forty-columns", 30, DirsMiddle, "~/a-directory-na…forty-columns"},
		{"a-directory-name-far-too-long-to-fit", 20, DirsMiddle, "a-director…ng-to-fit"},

		{deep, 40, DirsFish, "~/c/a/m/s/b/internal/ledger/testdata/v2"},
		{deep, 20, DirsFish, "~/c/a/m/s/b/i/l/t/v2"},
		{deepAbs, 30, DirsFish, "/s/b/w/.c/g/7f/objects/main"},
		{deepAbs, 20, DirsFish, "/s/b/w/.c/g/7/o/main"},
		{longLast, 40, DirsFish, "~/…/a-directory-name-f…-in-forty-columns"},
		{"~/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t", 20, DirsFish, "~/a/…/n/o/p/q/r/s/t"},

		{deep, 20, DirsFull, deep},
		{"~/src/api", 5, DirsMiddle, "…/api"},
	} {
		path, want := filepath.FromSlash(tc.path), filepath.FromSlash(tc.want)
		got := ShortDir(path, tc.limit, tc.how)
		if got != want {
			t.Errorf("%s %s in %d: got %q, want %q", tc.how, tc.path, tc.limit, got, want)
		}
		if tc.how != DirsFull && Width(got) > tc.limit {
			t.Errorf("%s %s in %d: %q is %d columns", tc.how, tc.path, tc.limit, got, Width(got))
		}
	}
}

func TestShortDirLeavesAPathThatFits(t *testing.T) {
	for _, how := range []string{DirsMiddle, DirsFish, DirsFull} {
		for _, path := range []string{"~", "~/src/api", "/", deep} {
			path = filepath.FromSlash(path)
			if got := ShortDir(path, Width(path), how); got != path {
				t.Errorf("%s: %q became %q", how, path, got)
			}
		}
	}
}

func TestShortDirKeepsWholeNamesButTheLast(t *testing.T) {
	path := filepath.FromSlash(deep)
	names := strings.Split(path, string(filepath.Separator))
	for limit := 10; limit < Width(path); limit++ {
		got := strings.Split(Middle(path, limit), string(filepath.Separator))
		for _, name := range got[:len(got)-1] {
			if name != "…" && !slices.Contains(names, name) {
				t.Errorf("limit %d: %q cuts inside %q", limit, Middle(path, limit), name)
			}
		}
	}
}

func TestCutMiddle(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"abcdefghij", 10, "abcdefghij"},
		{"abcdefghij", 5, "ab…ij"},
		{"abcdefghij", 6, "abc…ij"},
		{"日本語のディレクトリ", 9, "日本…トリ"},
		{"abc", 0, ""},
	} {
		if got := cutMiddle(tc.in, tc.limit); got != tc.want {
			t.Errorf("cutMiddle(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

func TestCutEndsWhatItShortensWithAnEllipsis(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"exactly 10", 10, "exactly 10"},
		{"one word too many", 10, "one word…"},
		{"日本語のテキスト", 7, "日本語…"},
	} {
		if got := cut(tc.in, tc.limit); got != tc.want || Width(got) > tc.limit {
			t.Errorf("cut(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

func TestFitGivesADirectoryTheRoomTheOtherColumnsLeave(t *testing.T) {
	path := filepath.FromSlash(deep)
	faint := func(s string) string { return "<" + s + ">" }
	for _, tc := range []struct {
		layout Layout
		want   string
	}{
		{Layout{Dirs: DirsMiddle}, Middle(path, MaxDir)},
		{Layout{Width: 200, Dirs: DirsMiddle}, path},
		// ID and NAME take 8 and 4 columns, and the gaps between columns 4.
		{Layout{Width: 46, Dirs: DirsMiddle}, Middle(path, 30)},
		{Layout{Width: 48, Margin: 2, Dirs: DirsMiddle}, Middle(path, 30)},
		{Layout{Width: 10, Dirs: DirsFish}, Fish(path, MinDir)},
		{Layout{Width: 10, Dirs: DirsFull}, path},
	} {
		rows := [][]string{{"ID", "DIR", "NAME"}, {"0123abcd", path, "api"}}
		l := tc.layout
		l.DirCols = []int{1}
		Fit(rows, l, faint)
		if rows[1][1] != "<"+tc.want+">" || rows[0][1] != "DIR" {
			t.Errorf("%+v: got %q, want %q", tc.layout, rows[1][1], tc.want)
		}
	}
}

func TestFitCutsTheLastColumnToWhatIsLeft(t *testing.T) {
	path, doing := filepath.FromSlash(deep), strings.Repeat("checking the build ", 5)
	plain := func(s string) string { return s }
	rows := [][]string{{"DIR", "DOING"}, {path, doing}}
	Fit(rows, Layout{Width: 50, Dirs: DirsMiddle, DirCols: []int{0}, FitLast: true}, plain)
	if got := Width(rows[1][0]); got > 50-2-MinLast {
		t.Errorf("DIR %q takes %d columns", rows[1][0], got)
	}
	if got := Width(rows[1][0]) + 2 + Width(rows[1][1]); got > 50 || Width(rows[1][1]) < MinLast-1 {
		t.Errorf("the line is %d columns: %q", got, rows[1])
	}
	rows = [][]string{{"DIR", "DOING"}, {path, doing}}
	Fit(rows, Layout{Dirs: DirsMiddle, DirCols: []int{0}, FitLast: true, LastCap: 20}, plain)
	if Width(rows[1][1]) > 20 || Width(rows[1][0]) > MaxDir {
		t.Errorf("off a terminal: %q", rows[1])
	}
}
