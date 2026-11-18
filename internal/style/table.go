package style

import (
	"io"
	"strings"
)

// Table writes rows in columns two spaces apart, as text/tabwriter does, but
// measures each cell without its colors, so colored cells line up. The last
// cell of a row is not padded.
func Table(w io.Writer, rows [][]string) error {
	widths := columnWidths(rows)
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-Width(cell)+2))
			}
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func columnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row[:max(0, len(row)-1)] {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], Width(cell))
		}
	}
	return widths
}

// indent is how many columns Table puts before the last cell of rows that all
// have the same number of cells.
func indent(rows [][]string) int {
	n := 0
	for _, w := range columnWidths(rows) {
		n += w + 2
	}
	return n
}

// MaxDir is the most columns a directory takes in a table that does not go to
// a terminal, and MinDir the fewest it is cut to on one that does.
const MaxDir, MinDir = 40, 20

// MinLast is the fewest columns a last column fitted to the terminal, such as
// DOING, gets.
const MinLast = 10

// Layout says how Fit fits a table to the line.
type Layout struct {
	// Width is the terminal's width, or 0 when the table does not go to one.
	Width int
	// Margin is how many columns go before each line.
	Margin int
	// Dirs is how to shorten a directory: DirsMiddle, DirsFish, or DirsFull.
	Dirs string
	// DirCols are the columns that hold a directory, which Fit shortens.
	DirCols []int
	// FitLast cuts the last column to what is left of the line after the
	// directories, or to LastCap off a terminal when LastCap is not 0.
	FitLast bool
	LastCap int
}

// Fit fits rows, the first being the header, to the line before Table writes
// them. It shortens each directory below the header to the room the other
// columns leave on the terminal's line, at least MinDir, or to MaxDir off a
// terminal, and paints it. With FitLast, a directory first gets that room as
// though the last column took only MinLast, and the last column then gets what
// is left.
func Fit(rows [][]string, l Layout, paint func(string) string) {
	widths := cellWidths(rows)
	for _, col := range l.DirCols {
		limit := MaxDir
		if l.Width > 0 {
			used := l.Margin + 2*(len(widths)-1)
			for i, w := range widths {
				switch {
				case i == col:
				case l.FitLast && i == len(widths)-1:
					used += MinLast
				default:
					used += w
				}
			}
			limit = DirRoom(l.Width, used)
		}
		for _, row := range rows[min(1, len(rows)):] {
			if col < len(row) {
				row[col] = paint(ShortDir(row[col], limit, l.Dirs))
			}
		}
	}
	if l.FitLast && len(rows) > 1 {
		limit := l.LastCap
		if l.Width > 0 {
			limit = max(l.Width-l.Margin-indent(rows), MinLast)
		}
		if limit > 0 {
			for _, row := range rows[1:] {
				row[len(row)-1] = cut(row[len(row)-1], limit)
			}
		}
	}
}

// DirRoom is how many columns a directory gets on a terminal width columns
// wide, when the rest of its line takes used, or MaxDir when width is 0.
func DirRoom(width, used int) int {
	if width <= 0 {
		return MaxDir
	}
	return max(width-used, MinDir)
}

// cellWidths is how wide each column of rows is, the last included.
func cellWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], Width(cell))
		}
	}
	return widths
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
