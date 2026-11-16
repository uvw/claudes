package cli

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
)

// pastSession is a session as claudes session shows it: what its transcript
// says, and whether it runs now.
type pastSession struct {
	ShortID string `json:"id"`
	transcripts.Session
	Running bool `json:"running"`
	// Disk is the space the session takes on disk.
	Disk transcripts.Disk `json:"disk"`
}

func (app *App) sessionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "session",
		Aliases: []string{"s", "sessions"},
		Short:   "Find, resume, and move past sessions, from their transcripts",
		Long: `Commands for every Claude Code session on this computer, past and running, read
from the transcripts under ~/.claude/projects. claudes only reads them, except
for session mv, which copies one, and session gc, which removes them.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(app.sessionLsCommand(), app.sessionDuCommand(), app.sessionShowCommand(), app.sessionWhereCommand(),
		alone(app.sessionResumeCommand()), alone(app.sessionMvCommand()), alone(reports(app.sessionGcCommand())))
	return cmd
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
			if s.Name != s.JobID {
				p.Name = cmp.Or(s.Name, p.Name)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func (app *App) completePast(single bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if single && len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		list, err := app.pastSessions(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var out []cobra.Completion
		for _, p := range list {
			state := "-"
			if p.Running {
				state = "running"
			}
			out = append(out, cobra.CompletionWithDesc(p.Label(),
				fmt.Sprintf("%s %s %s", p.ShortID, state, homeShort(p.Cwd))))
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
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
