package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/history"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/version"
)

// listed lists the sessions a command will act on, in the accounts it
// covers, and notes when.
func (app *App) listed(ctx context.Context, all bool) ([]claude.Session, error) {
	return app.listedIn(ctx, app.selected(), all)
}

// listedIn is listed for the sessions of accts.
func (app *App) listedIn(ctx context.Context, accts []*account, all bool) ([]claude.Session, error) {
	before := app.Now()
	sessions, err := app.listIn(ctx, accts, all)
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

// host is the session running claudes.
func (app *App) host() selection.Host { return selection.Host{ID: app.HostID, Job: app.HostJob} }

// pastSessions lists every transcript, newest activity first, and marks the
// running ones. A running session's name comes from agent view, which names
// some sessions its transcript does not.
func (app *App) pastSessions(ctx context.Context) ([]history.Session, error) {
	list, err := app.Transcripts.List()
	if err != nil {
		return nil, err
	}
	live := map[string]claude.Session{}
	app.runningKnown = false
	// Every account counts, since a past session can run under any of them.
	if sessions, err := app.listIn(ctx, app.Accounts, false); err == nil {
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
	out := make([]history.Session, 0, len(list))
	for i, t := range list {
		p := history.Session{ShortID: t.ID(), Session: t, Disk: disks[i]}
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
func (app *App) onePast(ctx context.Context, token string) (history.Session, error) {
	list, err := app.pastSessions(ctx)
	if err != nil {
		return history.Session{}, err
	}
	return history.Pick(token, list)
}

func (app *App) filterPast(list []history.Session, targets []string, in string, since time.Duration) ([]history.Session, error) {
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

// gcKeep is the sessions session gc must leave alone, each with why: running,
// listed by agent view in any account, parked, or the session running claudes.
func (app *App) gcKeep(ctx context.Context, list []history.Session) (map[string]string, error) {
	listed, err := app.listIn(ctx, app.Accounts, true)
	if err != nil {
		return nil, refusal(fmt.Errorf("cannot list agent view's sessions, so session gc removes nothing: %w", err))
	}
	entries, err := app.Park.Load()
	if err != nil {
		return nil, fmt.Errorf("cannot read the park file, so cannot tell which sessions are paused: %w", err)
	}
	parked := make([]string, len(entries))
	for i, e := range entries {
		parked[i] = e.SessionID
	}
	return history.Keep(list, listed, parked, app.host().IsID), nil
}

// midTurn is Session.MidTurn, plus a lead whose teammates are at work: its
// turn goes on through them, so a restart should ping it to carry on.
func (app *App) midTurn(s claude.Session) bool {
	return s.MidTurn() || len(app.teammates(s.SessionID)) > 0
}

// teamNote names the teammates that would stop with s, or is "".
func (app *App) teamNote(s claude.Session) string {
	names := app.teammates(s.SessionID)
	if len(names) == 0 {
		return ""
	}
	return fmt.Sprintf("leads %d teammate(s): %s, which stop with it and cannot be resumed", len(names), shell.PrintableJoin(names))
}

// warnTeams warns about named sessions whose teammates would stop with them.
func (app *App) warnTeams(sessions []claude.Session) {
	for _, s := range sessions {
		if note := app.teamNote(s); note != "" && s.Running() {
			app.warn("claudes: %s %s.", s.Label(), note)
		}
	}
}

// rows lists sessions for ls and where. installed is the installed
// Claude Code version, or "" when unknown, which marks no session outdated.
func (app *App) rows(ctx context.Context, all bool, installed string) ([]sessionRow, error) {
	sessions, err := app.listIn(ctx, app.selected(), all)
	if err != nil {
		return nil, err
	}
	pins := map[*account]map[string]bool{}
	for _, acct := range app.selected() {
		if pins[acct], err = acct.Jobs.PinSet(); err != nil {
			app.warn("claudes: cannot read pins: %v", err)
		}
	}
	rows := make([]sessionRow, 0, len(sessions))
	for _, s := range sessions {
		acct := app.accountOf(s)
		r := sessionRow{Session: s, Pinned: s.JobID != "" && pins[acct][s.JobID], Version: app.sessionVersion(s), Teammates: acct.Teams.Teammates(s.SessionID), Account: app.accountName(s)}
		r.Outdated = installed != "" && r.Version != "" && version.Less(r.Version, installed)
		a := acct.Jobs.Live(s.JobID)
		r.Subagents, r.Shells, r.Tokens, r.PRs, r.Doing = a.Subagents, a.Shells, a.Tokens, a.PRs, a.Doing
		if !s.Running() {
			// state.json can still list what ran when the process ended.
			r.Subagents, r.Shells = zeroIfKnown(r.Subagents), zeroIfKnown(r.Shells)
		}
		rows = append(rows, r)
	}
	sortRows(rows)
	return rows, nil
}
