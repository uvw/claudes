package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

// DirHeader heads a table's directory column, which Columns has Fit shorten
// and fade.
const DirHeader = "DIR"

// NameHeader and BranchHeader head the columns of NameCell and BranchCell,
// which Columns has Fit cut further when a line is wider than the terminal.
const NameHeader, BranchHeader = "NAME", "BRANCH"

// Dash is s, or "-" for a value a table cell does not have.
func Dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Age is how long before now t was, such as 3d, or "-" when t is unknown.
func Age(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// Ago is how long ago t was, such as "3d ago", or "now", or "unknown".
func Ago(now, t time.Time) string {
	switch a := Age(now, t); a {
	case "now":
		return a
	case "-":
		return "unknown"
	default:
		return a + " ago"
	}
}

// MaxName is the most characters of a name a table shows. Claude Code's own
// titles for sessions run past 60. On a terminal too narrow for the line, Fit
// cuts a name to as few as MinName.
const MaxName, MinName = 40, 12

// NameCell is a name as a table shows it, in a column headed NameHeader:
// printable, on one line, and cut to MaxName characters.
func NameCell(name string) string { return FirstLine(shell.Printable(name), MaxName) }

// MaxBranch is the most columns of a git branch a table shows, enough for
// feat/ and a few words. On a terminal too narrow for the line, Fit cuts a
// branch to as few as MinBranch.
const MaxBranch, MinBranch = 24, 12

// BranchCell is a git branch as a table shows it, in a column headed
// BranchHeader: printable, and cut in the middle to MaxBranch columns, or "-"
// when there is none. Every branch a table shows goes through it.
func BranchCell(branch string) string {
	return Dash(Branch(shell.Printable(branch), MaxBranch))
}

// DirCell is a table cell that shows the directory path, in a column headed
// DirHeader. Every path a table shows goes through it.
func DirCell(path string) string { return HomeShort(path) }

// HomeShort is path, printable, with ~ for the home directory.
func HomeShort(path string) string {
	path = shell.Printable(path)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, ok := paths.RelWithin(home, path)
	switch {
	case !ok:
		return path
	case rel == ".":
		return "~"
	}
	return "~" + string(filepath.Separator) + rel
}

// FirstLine is the first line of s, cut to at most limit characters.
func FirstLine(s string, limit int) string {
	s, _, cut := strings.Cut(s, "\n")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	if cut {
		return s + " …"
	}
	return s
}

// CountCell is a count, or "-" when it is unknown or 0.
func CountCell(n *int) string {
	if n == nil || *n == 0 {
		return "-"
	}
	return strconv.Itoa(*n)
}

// Tokens is a token count as agent view prints it, such as 1.4M, or "-" when
// it is unknown.
func Tokens(n *int64) string {
	if n == nil {
		return "-"
	}
	if *n < 1000 {
		return strconv.FormatInt(*n, 10)
	}
	v, unit := float64(*n), ""
	for _, unit = range []string{"K", "M", "B"} {
		// Moves on at 999.5, which would round up to 1000.
		if v /= 1000; v < 999.5 {
			break
		}
	}
	if v < 9.95 {
		return fmt.Sprintf("%.1f%s", v, unit)
	}
	return fmt.Sprintf("%.0f%s", v, unit)
}
