// Package style colors what claudes prints, when it prints to a terminal.
package style

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Palette styles text for one output stream. The zero Palette leaves text
// plain.
type Palette struct{ on bool }

// On is a Palette that colors whatever the stream is.
var On = Palette{on: true}

// Detect decides whether text written to w gets colors. env is the
// environment, as os.Environ returns it. It follows the clicolors convention
// (https://bixense.com/clicolors/), where an empty variable counts as unset.
// In that order, NO_COLOR turns colors off, CLICOLOR_FORCE but 0 turns them on
// even when w is not a terminal, and CLICOLOR=0 turns them off. Otherwise w
// gets colors when it is a terminal that shows them.
func Detect(w io.Writer, env []string) Palette {
	if lookup(env, "NO_COLOR") != "" {
		return Palette{}
	}
	if force := lookup(env, "CLICOLOR_FORCE"); force == "" || force == "0" {
		if lookup(env, "CLICOLOR") == "0" || !isTerminal(w) || !shows(w, env) {
			return Palette{}
		}
	}
	if f, ok := w.(*os.File); ok {
		// The console in Windows 10 and later shows colors only once asked to.
		lipgloss.EnableLegacyWindowsANSI(f)
	}
	return On
}

// isTerminal reports whether w is a terminal. Tests replace it.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// shows reports whether the terminal w shows colors, such as one with
// TERM=dumb does not. Detect settles CLICOLOR_FORCE before it asks, and
// colorprofile's other way to force colors on a pipe, TTY_FORCE, is left out.
func shows(w io.Writer, env []string) bool {
	kept := []string{"TTY_FORCE=1"}
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k != "CLICOLOR_FORCE" && k != "TTY_FORCE" {
			kept = append(kept, kv)
		}
	}
	return colorprofile.Detect(w, kept) >= colorprofile.ANSI
}

// lookup is the last value env gives key, or "".
func lookup(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}

// Colors reports whether p styles text.
func (p Palette) Colors() bool { return p.on }

var (
	red     = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Red)
	green   = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Green)
	yellow  = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Yellow)
	blue    = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.BrightBlue)
	magenta = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Magenta)
	cyan    = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Cyan)
	faint   = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Faint(true)
)

func (p Palette) render(s lipgloss.Style, text string) string {
	if !p.on || text == "" {
		return text
	}
	// Each line on its own, since Render pads the lines of a block to one
	// width and turns tabs into spaces.
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = s.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// Red is for failures and errors.
func (p Palette) Red(text string) string { return p.render(red, text) }

// Green is for what worked or is at work.
func (p Palette) Green(text string) string { return p.render(green, text) }

// Yellow is for warnings and what is out of date.
func (p Palette) Yellow(text string) string { return p.render(yellow, text) }

// Blue marks a choice the user made, such as a pin.
func (p Palette) Blue(text string) string { return p.render(blue, text) }

// Magenta is for a session that waits on the user.
func (p Palette) Magenta(text string) string { return p.render(magenta, text) }

// Cyan is for a session at a shell prompt.
func (p Palette) Cyan(text string) string { return p.render(cyan, text) }

// Faint is for detail the eye can skip, such as a directory.
func (p Palette) Faint(text string) string { return p.render(faint, text) }

// Width is how many columns text takes in a terminal, without its colors.
func Width(text string) int { return lipgloss.Width(text) }

// Pad styles text with paint and pads it with spaces to width columns, so a
// colored word lines up as a plain one would.
func Pad(text string, width int, paint func(string) string) string {
	return paint(text) + strings.Repeat(" ", max(0, width-Width(text)))
}

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

// Indent is how many columns Table puts before the last cell of rows that all
// have the same number of cells.
func Indent(rows [][]string) int {
	n := 0
	for _, w := range columnWidths(rows) {
		n += w + 2
	}
	return n
}

// dirMark starts a table cell that holds a directory, for Fit to find.
// shell.Printable drops a NUL, so no other cell starts with one.
const dirMark = "\x00dir\x00"

// MaxDir is the most columns a directory takes in a table that does not go to
// a terminal, and MinDir the fewest it is cut to on one that does.
const MaxDir, MinDir = 40, 20

// MinLast is the fewest columns a last column fitted to the terminal, such as
// DOING, gets.
const MinLast = 10

// DirCell is a table cell that shows the directory path, which Fit shortens.
func DirCell(path string) string { return dirMark + path }

// Layout says how Fit fits a table to the line.
type Layout struct {
	// Width is the terminal's width, or 0 when the table does not go to one.
	Width int
	// Margin is how many columns go before each line.
	Margin int
	// Dirs is how to shorten a directory: DirsMiddle, DirsFish, or DirsFull.
	Dirs string
	// FitLast cuts the last column to what is left of the line after the
	// directories, or to LastCap off a terminal when LastCap is not 0.
	FitLast bool
	LastCap int
}

// Fit fits rows, the first being the header, to the line before Table writes
// them. It shortens each DirCell to the room the other columns leave on the
// terminal's line, at least MinDir, or to MaxDir off a terminal, and paints
// it. With FitLast, a directory first gets that room as though the last column
// took only MinLast, and the last column then gets what is left.
func Fit(rows [][]string, l Layout, paint func(string) string) {
	dirs := map[int][]int{}
	for i, row := range rows {
		for j, cell := range row {
			if path, ok := strings.CutPrefix(cell, dirMark); ok {
				row[j] = path
				dirs[j] = append(dirs[j], i)
			}
		}
	}
	widths := cellWidths(rows)
	for col, at := range dirs {
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
		for _, i := range at {
			rows[i][col] = paint(ShortDir(rows[i][col], limit, l.Dirs))
		}
	}
	if l.FitLast && len(rows) > 1 {
		limit := l.LastCap
		if l.Width > 0 {
			limit = max(l.Width-l.Margin-Indent(rows), MinLast)
		}
		if limit > 0 {
			for _, row := range rows[1:] {
				row[len(row)-1] = Cut(row[len(row)-1], limit)
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

// Cut shortens s to at most limit columns, ending it with … when it shortens.
func Cut(s string, limit int) string {
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

// TerminalWidth is how many columns the terminal w writes to has, or 0 when w
// is not a terminal.
func TerminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok || !isTerminal(w) {
		return 0
	}
	width, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0
	}
	return width
}

// Size is a byte count as du -h prints it: 512B, 12K, 3.4M, or 1.2G, in
// powers of 1024.
func Size(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	value, suffix := float64(n), ""
	for _, s := range []string{"K", "M", "G", "T"} {
		value /= unit
		suffix = s
		if value < unit {
			break
		}
	}
	if value < 10 {
		return fmt.Sprintf("%.1f%s", value, suffix)
	}
	return fmt.Sprintf("%.0f%s", value, suffix)
}
