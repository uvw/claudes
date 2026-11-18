package render

import (
	"slices"
	"strings"
)

// MaxDir is the most columns a directory takes in a table that does not go to
// a terminal, and MinDir the fewest it is cut to on one that does.
const MaxDir, MinDir = 40, 20

// MinLast is the fewest columns a last column fitted to the terminal, such as
// DOING, gets.
const MinLast = 10

// Cut is how Fit shortens the cells of a column.
type Cut int

const (
	// CutEnd keeps the start of a cell and ends it with …, as for a name.
	CutEnd Cut = iota
	// CutBranch puts … in the middle of a cell, as Branch does.
	CutBranch
	// CutDir shortens a directory as Layout.Dirs says, and paints it.
	CutDir
)

// Column is a column Fit may cut.
type Column struct {
	Index int
	Cut   Cut
	// Min is the fewest columns it is cut to on a terminal.
	Min int
	// Max is the most columns it takes off a terminal, or 0 for no limit.
	Max int
}

// Layout says how Fit fits a table to the line.
type Layout struct {
	// Width is the terminal's width, or 0 when the table does not go to one.
	Width int
	// Margin is how many columns go before each line.
	Margin int
	// Dirs is how to shorten a directory: DirsMiddle, DirsFish, or DirsFull.
	Dirs string
	// Columns are the columns Fit may cut, in the order it cuts them.
	Columns []Column
}

// Columns are the columns Fit cuts in a table headed header, in the order it
// cuts them: the last column with fitLast, to lastCap off a terminal when
// lastCap is not 0, then each directory, then each name, then each branch.
func Columns(header []string, fitLast bool, lastCap int) []Column {
	var cols []Column
	last := len(header) - 1
	if fitLast {
		cols = append(cols, Column{Index: last, Cut: CutEnd, Min: MinLast, Max: lastCap})
	}
	for _, c := range []struct {
		header string
		col    Column
	}{
		{DirHeader, Column{Cut: CutDir, Min: MinDir, Max: MaxDir}},
		{NameHeader, Column{Cut: CutEnd, Min: MinName}},
		{BranchHeader, Column{Cut: CutBranch, Min: MinBranch}},
	} {
		for i, h := range header {
			if h == c.header && (i != last || !fitLast) {
				c.col.Index = i
				cols = append(cols, c.col)
			}
		}
	}
	return cols
}

// Fit fits rows, the first being the header, to the line before Table writes
// them, and paints each directory below the header. Off a terminal, it cuts
// each column to its Max. On one, while the line is too wide, it cuts each
// column in turn down to its Min before it cuts the next. Room a cut leaves
// unused, as when a directory keeps whole names, first shortens a line still
// too wide with every column at its Min, and the rest goes to the columns cut
// before it.
func Fit(rows [][]string, l Layout, paint func(string) string) {
	if len(rows) == 0 {
		return
	}
	widths := cellWidths(rows)
	cols := slices.DeleteFunc(slices.Clone(l.Columns), func(c Column) bool { return c.Index < 0 || c.Index >= len(widths) })
	limits := make([]int, len(cols))
	for i, c := range cols {
		limits[i] = c.Max
	}
	over := 0
	if l.Width > 0 {
		over = l.Margin + 2*(len(widths)-1) - l.Width
		for _, w := range widths {
			over += w
		}
		for i, c := range cols {
			floor := max(c.Min, Width(rows[0][c.Index]))
			if c.Cut == CutDir && l.Dirs == DirsFull {
				floor = widths[c.Index]
			}
			give := min(max(over, 0), max(widths[c.Index]-floor, 0))
			limits[i], over = widths[c.Index]-give, over-give
		}
	}
	spare := 0
	for i := len(cols) - 1; i >= 0; i-- {
		c := cols[i]
		limit := limits[i] + spare
		for _, row := range rows[1:] {
			if c.Index < len(row) {
				row[c.Index] = shorten(row[c.Index], limit, c.Cut, l.Dirs)
			}
		}
		if l.Width > 0 {
			spare = max(limit-columnWidth(rows, c.Index), 0)
			paid := min(spare, max(over, 0))
			spare, over = spare-paid, over-paid
		}
	}
	for _, c := range cols {
		if c.Cut == CutDir {
			for _, row := range rows[1:] {
				if c.Index < len(row) {
					row[c.Index] = paint(row[c.Index])
				}
			}
		}
	}
}

// shorten cuts s to at most limit columns, as how says, or leaves it when
// limit is 0.
func shorten(s string, limit int, how Cut, dirs string) string {
	switch {
	case limit == 0:
		return s
	case how == CutDir:
		return ShortDir(s, limit, dirs)
	case how == CutBranch:
		return Branch(s, limit)
	}
	return cut(s, limit)
}

// DirRoom is how many columns a directory gets on a terminal width columns
// wide, when the rest of its line takes used, or MaxDir when width is 0.
func DirRoom(width, used int) int {
	if width <= 0 {
		return MaxDir
	}
	return max(width-used, MinDir)
}

// cut shortens s to at most limit columns, ending it with … when it shortens.
func cut(s string, limit int) string {
	s = strings.TrimSpace(s)
	if Width(s) <= limit {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && Width(string(r))+1 > limit {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "…"
}

// Branch shortens a git branch to at most limit columns by putting … in its
// middle. It keeps a first name that ends in a slash, such as feat/, when that
// takes at most half of limit, then as much of the end as fits, from the start
// of a word when that costs few columns: feat/ledger-rounding-for-every-currency
// cut to 24 columns reads feat/…for-every-currency.
func Branch(name string, limit int) string {
	if Width(name) <= limit {
		return name
	}
	first, rest, ok := strings.Cut(name, "/")
	if !ok || first == "" || Width(first)+1 > (limit-1)/2 {
		return cutMiddle(name, limit)
	}
	return first + "/" + endOf(rest, limit-Width(first)-1)
}

// endOf is … and the end of s, in at most limit columns. When the end starts
// inside a word, it drops what comes before the next -, _, /, or . if that
// leaves at least two thirds of the end.
func endOf(s string, limit int) string {
	r := []rune(s)
	start, room := len(r), limit-1
	for start > 0 && Width(string(r[start-1])) <= room {
		room -= Width(string(r[start-1]))
		start--
	}
	if start > 0 && !isWordBreak(r[start-1]) {
		end := r[start:]
		for i, c := range end[:len(end)/3] {
			if isWordBreak(c) {
				start += i + 1
				break
			}
		}
	}
	return "…" + string(r[start:])
}

func isWordBreak(r rune) bool { return strings.ContainsRune("-_/.", r) }
