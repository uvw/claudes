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
		{deep, 40, DirsMiddle, "~/…/billing/internal/ledger/testdata/v2"},
		{deep, 30, DirsMiddle, "~/code/…/ledger/testdata/v2"},
		{deep, 20, DirsMiddle, "~/code/…/testdata/v2"},
		{deepAbs, 40, DirsMiddle, "/srv/…/.cache/go-build/7f/objects/main"},
		{deepAbs, 20, DirsMiddle, "/…/7f/objects/main"},
		{longLast, 40, DirsMiddle, "~/…/a-directory-name-f…-in-forty-columns"},
		{"~/a-directory-name-far-too-long-to-fit-in-forty-columns", 30, DirsMiddle, "~/a-directory-na…forty-columns"},
		{"a-directory-name-far-too-long-to-fit", 20, DirsMiddle, "a-director…ng-to-fit"},

		{deep, 40, DirsFish, "~/c/a/m/s/b/internal/ledger/testdata/v2"},
		{deep, 20, DirsFish, "~/c/a/m/s/b/i/l/t/v2"},
		{deepAbs, 30, DirsFish, "/s/b/w/.c/g/7f/objects/main"},
		{deepAbs, 20, DirsFish, "/s/b/w/.c/g/7/o/main"},
		{longLast, 40, DirsFish, "~/…/a-directory-name-f…-in-forty-columns"},
		{"~/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t", 20, DirsFish, "~/…/m/n/o/p/q/r/s/t"},

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
