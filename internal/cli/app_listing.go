package cli

import (
	"cmp"
	"slices"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
)

// sessionRow is a session as claudes ls and claudes where show it: with its
// pin, its Claude Code version, the team it leads, and what agent view counts
// for it now. Scripts read its JSON, so a key keeps its name.
type sessionRow struct {
	claude.Session
	Pinned bool `json:"pinned"`
	// Version is the Claude Code version the session last ran, when known.
	Version  string `json:"version"`
	Outdated bool   `json:"outdated"`
	// Teammates are the agent team members the session leads that run now.
	Teammates []string `json:"teammates"`
	// Subagents and Shells count the subagents and background shells the
	// session runs now. Tokens is agent view's running total of the tokens it
	// used, PRs the pull requests it opened, and Doing agent view's one-line
	// status. Each is null when unknown, as for an interactive session.
	Subagents *int    `json:"subagents"`
	Shells    *int    `json:"shells"`
	Tokens    *int64  `json:"tokens"`
	PRs       *int    `json:"prs"`
	Doing     *string `json:"doing"`
}

// sortRows orders rows pinned first, then busy, then newest.
func sortRows(rows []sessionRow) {
	slices.SortStableFunc(rows, func(a, b sessionRow) int {
		return cmp.Or(selection.TrueFirst(a.Pinned, b.Pinned), selection.TrueFirst(a.Busy(), b.Busy()), cmp.Compare(b.StartedAt, a.StartedAt))
	})
}

// doingCell is r's DOING cell, which the table cuts to fit.
func doingCell(r sessionRow) string {
	if r.Doing == nil {
		return "-"
	}
	return shell.Printable(*r.Doing)
}

// zeroIfKnown is 0 for a known count, and nil for an unknown one.
func zeroIfKnown(n *int) *int {
	if n == nil {
		return nil
	}
	return new(int)
}
