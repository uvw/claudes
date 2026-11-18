package style

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBranchKeepsItsPrefixAndItsEnd(t *testing.T) {
	for _, tc := range []struct {
		in    string
		limit int
		want  string
	}{
		{"main", 24, "main"},
		{"feat/invoice-export-to-csv", 26, "feat/invoice-export-to-csv"},
		{"feat/invoice-export-to-csv", 24, "feat/…export-to-csv"},
		{"feat/invoice-export-to-csv", 12, "feat/…to-csv"},
		{"fix/averyveryverylongword-x", 20, "fix/…yverylongword-x"},
		{"release/2026.11/hotfix-ledger-totals", 24, "release/…ledger-totals"},
		// A first name of half the room or more gives way to a middle cut.
		{"dependabot/go_modules/golang.org/x/sys-0.40.0", 20, "dependabot…ys-0.40.0"},
		{"a-very-long-branch-with-no-slash", 16, "a-very-l…o-slash"},
		{"feat/日本語のブランチ名", 16, "feat/…ブランチ名"},
	} {
		if got := Branch(tc.in, tc.limit); got != tc.want || Width(got) > tc.limit {
			t.Errorf("Branch(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
		}
	}
}

// A directory that Middle cuts well short of MinDir, as a Windows temporary
// path can be, frees room the line needs before the last column gets any.
func TestFitSpendsUnusedRoomOnTheLineFirst(t *testing.T) {
	path := filepath.FromSlash("~/abcdefghijklmnop/qrstuvwxyz0123456789/x")
	doing := strings.Repeat("checking the build ", 5)
	for _, tc := range []struct{ width, doing int }{{10, MinLast}, {20, MinLast + 3}} {
		rows := [][]string{{"DIR", "DOING"}, {path, doing}}
		Fit(rows, Layout{Width: tc.width, Dirs: DirsMiddle, Columns: []Column{lastColumn(1, 0), dirColumn(0)}}, func(s string) string { return s })
		// cut drops the space before its …, so a cell can end a column short.
		if w := Width(rows[1][1]); w > tc.doing || w < tc.doing-1 {
			t.Errorf("width %d: DOING %q is %d columns, want %d", tc.width, rows[1][1], w, tc.doing)
		}
	}
}

// fitRow fits a row of ID, BRANCH, DIR, and NAME to width, cutting DIR, then
// NAME, then BRANCH, and returns how wide each column is and the line.
func fitRow(width int, dirs string) (branch, dir, name, line int) {
	rows := [][]string{
		{"ID", "BRANCH", "DIR", "NAME"},
		{"0123abcd", "feat/invoice-export-to-csv", filepath.FromSlash(deep), strings.Repeat("a long name ", 4)[:40]},
	}
	Fit(rows, Layout{Width: width, Dirs: dirs, Columns: []Column{
		dirColumn(2), {Index: 3, Cut: CutEnd, Min: 12}, {Index: 1, Cut: CutBranch, Min: 12},
	}}, func(s string) string { return s })
	branch, dir, name = columnWidth(rows, 1), columnWidth(rows, 2), columnWidth(rows, 3)
	return branch, dir, name, 8 + branch + dir + name + 3*2
}

func TestFitCutsTheDirectoryThenTheNameThenTheBranch(t *testing.T) {
	branch0, dir0, name0, line0 := fitRow(200, DirsMiddle)
	if branch0 != 26 || dir0 != Width(deep) || name0 != 40 {
		t.Fatalf("a wide terminal cut a column: %d, %d, %d", branch0, dir0, name0)
	}
	// floors is the line a row takes once every column is at its floor.
	floors := 8 + 12 + MinDir + 12 + 3*2
	for width := 30; width < line0+5; width++ {
		branch, dir, name, line := fitRow(width, DirsMiddle)
		switch {
		case width >= floors && line > width:
			t.Errorf("width %d: the line is %d columns", width, line)
		case name < name0 && 8+branch+MinDir+name0+3*2 <= width:
			t.Errorf("width %d: NAME cut to %d, though DIR at its floor leaves it room", width, name)
		case branch < branch0 && 8+branch0+dir+12+3*2 <= width:
			t.Errorf("width %d: BRANCH cut to %d, though NAME at its floor leaves it room", width, branch)
		// cut drops the space before its …, so a name can end a column short.
		case name < 11 || branch < 12 || dir < MinDir-4:
			t.Errorf("width %d: a column below its floor: BRANCH %d, DIR %d, NAME %d", width, branch, dir, name)
		}
	}
	if branch, dir, name, _ := fitRow(0, DirsMiddle); branch != 26 || name != 40 || dir > MaxDir {
		t.Errorf("off a terminal: BRANCH %d, DIR %d, NAME %d", branch, dir, name)
	}
	// A directory output.dirs keeps whole leaves the cuts to the others.
	if branch, dir, name, line := fitRow(line0-20, DirsFull); dir != dir0 || name > 20 || name < 19 || branch != 26 || line > line0-20 {
		t.Errorf("full: BRANCH %d, DIR %d, NAME %d", branch, dir, name)
	}
}
