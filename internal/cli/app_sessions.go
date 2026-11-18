package cli

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/version"
)

// listed lists the sessions a command will act on and notes when.
func (app *App) listed(ctx context.Context, all bool) ([]claude.Session, error) {
	before := app.Now()
	sessions, err := app.Agents.Sessions(ctx, all)
	if err != nil {
		return nil, err
	}
	app.listedAt, app.lastList = before, sessions
	app.log().Debug("listed sessions", "all", all, "count", len(sessions))
	if err := app.compatible(ctx, sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

func (app *App) isHost(s claude.Session) bool {
	return app.isHostID(s.SessionID) || app.HostJob != "" && s.JobID == app.HostJob
}

// isHostID reports whether sessionID is the session running claudes.
func (app *App) isHostID(sessionID string) bool {
	return app.HostID != "" && sessionID == app.HostID ||
		app.HostJob != "" && claude.ShortID(sessionID) == app.HostJob
}

// hostLast moves the session running claudes to the end.
func (app *App) hostLast(sessions []claude.Session) []claude.Session {
	slices.SortStableFunc(sessions, func(a, b claude.Session) int {
		return trueFirst(!app.isHost(a), !app.isHost(b))
	})
	return sessions
}

// trueFirst orders a before b when a is true and b is not, for sorting.
func trueFirst(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return -1
	default:
		return 1
	}
}

// sessionRef is how the JSON of session du and session triage names a session.
type sessionRef struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

// Label is the session's name, or its short id when it has none.
func (r sessionRef) Label() string { return cmp.Or(r.Name, r.ID) }

// pastSession is a session as claudes session shows it: what its transcript
// says, and whether it runs now.
type pastSession struct {
	ShortID string `json:"id"`
	transcripts.Session
	Running bool `json:"running"`
	// Disk is the space the session takes on disk.
	Disk transcripts.Disk `json:"disk"`
	// Activity is what the session did, or nil when its transcript was not
	// read in full.
	Activity *transcripts.Tally `json:"activity"`
	// tallyErr is why reading the activity failed.
	tallyErr error
}

// readTally counts what p did, unless its transcript is bigger than
// transcripts.TallyLimit, or force reads it whatever its size.
func readTally(p *pastSession, force bool) {
	if p.Activity != nil || p.tallyErr != nil || !force && p.Size > transcripts.TallyLimit {
		return
	}
	a, err := transcripts.ReadTally(p.Path)
	if err != nil {
		p.tallyErr = err
		return
	}
	p.Activity = &a
}

func refOf(p pastSession) sessionRef {
	return sessionRef{ID: p.ShortID, SessionID: p.SessionID, Name: p.Name}
}

// pastSessions lists every transcript, newest activity first, and marks the
// running ones. A running session's name comes from agent view, which names
// some sessions its transcript does not.
func (app *App) pastSessions(ctx context.Context) ([]pastSession, error) {
	list, err := app.Transcripts.List()
	if err != nil {
		return nil, err
	}
	live := map[string]claude.Session{}
	app.runningKnown = false
	if sessions, err := app.Agents.Sessions(ctx, false); err == nil {
		app.runningKnown = true
		for _, s := range sessions {
			if s.Running() {
				live[s.SessionID] = s
			}
		}
	} else {
		app.warn("claudes: cannot tell which sessions are running: %v", err)
	}
	disks := app.Transcripts.Measure(list)
	out := make([]pastSession, 0, len(list))
	for i, t := range list {
		p := pastSession{ShortID: t.ID(), Session: t, Disk: disks[i]}
		if s, ok := live[t.SessionID]; ok {
			p.Running = true
			// claude agents names an unnamed session by its job id.
			if s.Name != s.JobID && s.Name != "" && s.Name != p.Name {
				p.Name, p.NameSource = s.Name, transcripts.NameAgent
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// onePast resolves a target to exactly one session.
func (app *App) onePast(ctx context.Context, token string) (pastSession, error) {
	list, err := app.pastSessions(ctx)
	if err != nil {
		return pastSession{}, err
	}
	return pickPast(token, list)
}

// pickPast resolves token to one of list, which is newest first. Sessions
// reuse names over time, so a token that is exactly a name picks the newest
// session with that name. Otherwise the token must name one session, as
// target.Resolve decides.
func pickPast(token string, list []pastSession) (pastSession, error) {
	for _, p := range list {
		if p.Name != "" && p.Name == token {
			return p, nil
		}
	}
	picked, err := target.Resolve([]string{token}, list)
	if err == nil {
		return picked[0], nil
	}
	if matches, merr := target.Match([]string{token}, list); merr == nil && (len(matches) == 1 || sameName(matches)) {
		return matches[0], nil
	}
	return pastSession{}, err
}

// sameName reports whether every session in list has the same, non-empty name.
func sameName(list []pastSession) bool {
	if len(list) == 0 || list[0].Name == "" {
		return false
	}
	for _, p := range list[1:] {
		if p.Name != list[0].Name {
			return false
		}
	}
	return true
}

func (app *App) filterPast(list []pastSession, targets []string, in string, since time.Duration) ([]pastSession, error) {
	if len(targets) > 0 {
		var err error
		if list, err = listMatch(app, targets, list); err != nil {
			return nil, err
		}
	}
	var root string
	if in != "" {
		var err error
		if root, err = filepath.Abs(in); err != nil {
			return nil, err
		}
		// Sessions record the real directory, so a link in --in is followed.
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
	}
	out := list[:0]
	for _, p := range list {
		if root != "" && !paths.Within(p.Cwd, root) && !paths.Within(paths.Resolved(p.Cwd), root) {
			continue
		}
		if since > 0 && app.Now().Sub(p.LastActive) > since {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// midTurn is Session.MidTurn, plus a lead whose teammates are at work: its
// turn goes on through them, so a restart should ping it to carry on.
func (app *App) midTurn(s claude.Session) bool {
	return s.MidTurn() || len(app.Teams.Teammates(s.SessionID)) > 0
}

// teamNote names the teammates that would stop with s, or is "".
func (app *App) teamNote(s claude.Session) string {
	names := app.Teams.Teammates(s.SessionID)
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("leads %d teammate(s): %s, which stop with it and cannot be resumed", len(names), printableList(names))
}

// warnTeams warns about named sessions whose teammates would stop with them.
func (app *App) warnTeams(sessions []claude.Session) {
	for _, s := range sessions {
		if note := app.teamNote(s); note != "" && s.Running() {
			app.warn("claudes: %s %s.", s.Label(), note)
		}
	}
}

// printableList joins names read from Claude Code's files, with any control
// characters in them made visible.
func printableList(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = shell.Printable(n)
	}
	return strings.Join(out, ", ")
}

func joinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

// row is a session as `claudes ls` shows it.
type row struct {
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

// rows lists sessions for ls and where. installed is the installed
// Claude Code version, or "" when unknown, which marks no session outdated.
func (app *App) rows(ctx context.Context, all bool, installed string) ([]row, error) {
	sessions, err := app.Agents.Sessions(ctx, all)
	if err != nil {
		return nil, err
	}
	pins, err := app.Jobs.PinSet()
	if err != nil {
		app.warn("claudes: cannot read pins: %v", err)
	}
	rows := make([]row, 0, len(sessions))
	for _, s := range sessions {
		r := row{Session: s, Pinned: s.JobID != "" && pins[s.JobID], Version: app.sessionVersion(s), Teammates: app.Teams.Teammates(s.SessionID)}
		r.Outdated = installed != "" && r.Version != "" && version.Less(r.Version, installed)
		a := app.Jobs.Live(s.JobID)
		r.Subagents, r.Shells, r.Tokens, r.PRs, r.Doing = a.Subagents, a.Shells, a.Tokens, a.PRs, a.Doing
		if !s.Running() {
			// state.json can still list what ran when the process ended.
			r.Subagents, r.Shells = zeroIfKnown(r.Subagents), zeroIfKnown(r.Shells)
		}
		rows = append(rows, r)
	}
	slices.SortStableFunc(rows, func(a, b row) int {
		return cmp.Or(trueFirst(a.Pinned, b.Pinned), trueFirst(a.Busy(), b.Busy()), cmp.Compare(b.StartedAt, a.StartedAt))
	})
	return rows, nil
}

// zeroIfKnown is 0 for a known count, and nil for an unknown one.
func zeroIfKnown(n *int) *int {
	if n == nil {
		return nil
	}
	return new(int)
}
