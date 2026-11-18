package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
)

// dash is s, or "-" for a value a table cell does not have.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// age is how long before now t was, such as 3d, or "-" when t is unknown.
func age(now, t time.Time) string {
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

// ago is how long ago t was, such as "3d ago", or "now", or "unknown".
func ago(now, t time.Time) string {
	switch a := age(now, t); a {
	case "now":
		return a
	case "-":
		return "unknown"
	default:
		return a + " ago"
	}
}

// maxName is the most characters of a name a table shows. Claude Code's own
// titles for sessions run past 60. On a terminal too narrow for the line,
// writeTable cuts a name to as few as minName.
const maxName, minName = 40, 12

// nameCell is a name as a table shows it, in a column headed nameHeader:
// printable, on one line, and cut to maxName characters.
func nameCell(name string) string { return firstLine(shell.Printable(name), maxName) }

// maxBranch is the most columns of a git branch a table shows, enough for
// feat/ and a few words. On a terminal too narrow for the line, writeTable
// cuts a branch to as few as minBranch.
const maxBranch, minBranch = 24, 12

// branchCell is a git branch as a table shows it, in a column headed
// branchHeader: printable, and cut in the middle to maxBranch columns, or "-"
// when there is none. Every branch a table shows goes through it.
func branchCell(branch string) string {
	return dash(style.Branch(shell.Printable(branch), maxBranch))
}

// firstLine is the first line of s, cut to at most limit characters.
func firstLine(s string, limit int) string {
	s, _, cut := strings.Cut(s, "\n")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	if cut {
		return s + " …"
	}
	return s
}
