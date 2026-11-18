// Package listing holds the rows claudes ls and claudes where print and
// encode: each session with its pin, its Claude Code version, the team it
// leads, and what agent view counts for it now.
package listing

import (
	"cmp"
	"slices"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
)

// Row is a session as claudes ls shows it. Scripts read its JSON, so a key
// keeps its name.
type Row struct {
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

// Sort orders rows pinned first, then busy, then newest.
func Sort(rows []Row) {
	slices.SortStableFunc(rows, func(a, b Row) int {
		return cmp.Or(selection.TrueFirst(a.Pinned, b.Pinned), selection.TrueFirst(a.Busy(), b.Busy()), cmp.Compare(b.StartedAt, a.StartedAt))
	})
}

// Doing is r's DOING cell, which the table cuts to fit.
func Doing(r Row) string {
	if r.Doing == nil {
		return "-"
	}
	return shell.Printable(*r.Doing)
}

// ZeroIfKnown is 0 for a known count, and nil for an unknown one.
func ZeroIfKnown(n *int) *int {
	if n == nil {
		return nil
	}
	return new(int)
}
