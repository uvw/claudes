package cli

import (
	"cmp"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

// dirMark starts a table cell that holds a directory, for writeTable to find.
// shell.Printable drops a NUL, so no other cell starts with one.
const dirMark = "\x00dir\x00"

// maxDir is the most columns a directory takes in a table that does not go to
// a terminal, and minDir the fewest it is cut to on one that does.
const maxDir, minDir = 40, 20

// minLast is the fewest columns a last column fitted to the terminal, such as
// DOING, gets.
const minLast = 10

// dirCell is a table cell that shows the directory path. Every path a table
// shows goes through it, so writeTable can shorten and fade it.
func dirCell(path string) string { return dirMark + path }

// tableFit says how writeTable fits a table to the line.
type tableFit struct {
	// margin is how many columns go before each line.
	margin int
	// fitLast cuts the last column to what is left of the terminal's width
	// after the directories, or to lastCap when the table does not go to a
	// terminal and lastCap is not 0.
	fitLast bool
	lastCap int
}

// writeTable writes rows as style.Table does, the first being the header. It
// shortens each directory cell, made by dirCell, to the room the other columns
// leave on the terminal's line, or to maxDir when the table does not go to a
// terminal, and fades it. With fitLast, a directory first gets that room as
// though the last column took only minLast, and the last column then gets what
// is left.
func (app *App) writeTable(w io.Writer, rows [][]string, fit tableFit) error {
	width := app.width()
	dirs := map[int][]int{}
	for i, row := range rows {
		for j, cell := range row {
			if path, ok := strings.CutPrefix(cell, dirMark); ok {
				row[j] = homeShort(path)
				dirs[j] = append(dirs[j], i)
			}
		}
	}
	widths := cellWidths(rows)
	how := cmp.Or(app.Config.Output.Dirs, style.DirsMiddle)
	for col, at := range dirs {
		limit := maxDir
		if width > 0 {
			used := fit.margin + 2*(len(widths)-1)
			for i, w := range widths {
				switch {
				case i == col:
				case fit.fitLast && i == len(widths)-1:
					used += minLast
				default:
					used += w
				}
			}
			limit = max(width-used, minDir)
		}
		for _, i := range at {
			rows[i][col] = app.colors.Faint(style.ShortDir(rows[i][col], limit, how))
		}
	}
	if fit.fitLast && len(rows) > 1 {
		limit := fit.lastCap
		if width > 0 {
			limit = max(width-fit.margin-style.Indent(rows), minLast)
		}
		if limit > 0 {
			for _, row := range rows[1:] {
				row[len(row)-1] = cut(row[len(row)-1], limit)
			}
		}
	}
	return style.Table(w, rows)
}

// cellWidths is how wide each column of rows is, the last included.
func cellWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], style.Width(cell))
		}
	}
	return widths
}

// dirIn is path as a line of text shows it, such as a heading, shortened as
// writeTable shortens a directory. used is how many columns the rest of the
// line takes.
func (app *App) dirIn(path string, used int) string {
	limit := maxDir
	if width := app.width(); width > 0 {
		limit = max(width-used, minDir)
	}
	return style.ShortDir(homeShort(path), limit, cmp.Or(app.Config.Output.Dirs, style.DirsMiddle))
}

// width is how many columns the terminal Out writes to has, or 0 when Out is
// not a terminal.
func (app *App) width() int {
	if app.Width == nil {
		return 0
	}
	return max(app.Width(), 0)
}

// homeShort is path, printable, with ~ for the home directory.
func homeShort(path string) string {
	path = shell.Printable(path)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, ok := relWithin(home, path)
	switch {
	case !ok:
		return path
	case rel == ".":
		return "~"
	}
	return "~" + string(filepath.Separator) + rel
}
