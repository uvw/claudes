package render

import (
	"path/filepath"
	"slices"
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
		l.Columns = []Column{dirColumn(1)}
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
	Fit(rows, Layout{Width: 50, Dirs: DirsMiddle, Columns: []Column{lastColumn(1, 0), dirColumn(0)}}, plain)
	if got := Width(rows[1][0]); got > 50-2-MinLast {
		t.Errorf("DIR %q takes %d columns", rows[1][0], got)
	}
	if got := Width(rows[1][0]) + 2 + Width(rows[1][1]); got > 50 || Width(rows[1][1]) < MinLast-1 {
		t.Errorf("the line is %d columns: %q", got, rows[1])
	}
	rows = [][]string{{"DIR", "DOING"}, {path, doing}}
	Fit(rows, Layout{Dirs: DirsMiddle, Columns: []Column{lastColumn(1, 20), dirColumn(0)}}, plain)
	if Width(rows[1][1]) > 20 || Width(rows[1][0]) > MaxDir {
		t.Errorf("off a terminal: %q", rows[1])
	}
}

func TestColumnsCutTheLastThenDirsThenNamesThenBranches(t *testing.T) {
	header := []string{"ID", BranchHeader, DirHeader, NameHeader, "DOING"}
	got := Columns(header, true, 30)
	want := []Column{
		{Index: 4, Cut: CutEnd, Min: MinLast, Max: 30},
		{Index: 2, Cut: CutDir, Min: MinDir, Max: MaxDir},
		{Index: 3, Cut: CutEnd, Min: MinName},
		{Index: 1, Cut: CutBranch, Min: MinBranch},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// A name in the last column is cut as the last column, once.
	if got := Columns([]string{"ID", NameHeader}, true, 0); len(got) != 1 || got[0].Index != 1 || got[0].Min != MinLast {
		t.Errorf("a last NAME: %+v", got)
	}
}
