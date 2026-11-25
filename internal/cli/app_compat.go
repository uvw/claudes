package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/compat"
)

// compatible warns once when the installed Claude Code is outside the tested
// range, and refuses sessions claudes does not recognize unless --force.
func (app *App) compatible(ctx context.Context, sessions []claude.Session) error {
	app.warnVersion(ctx)
	return app.refuse("claude agents --json", compat.CheckSessions(sessions))
}

// protectedPins reads the pinned set that bulk commands must leave alone, from
// the pins.json of each account the command covers. It refuses, unless
// --force, a pins.json that holds anything but job ids, and a jobs directory
// that is missing while its account's sessions have job ids: either means
// claudes may be reading the wrong file, and every pinned session would look
// unpinned. The set joins every account's job ids, so a job id pinned in one
// account protects a session with the same id in another, never the reverse.
func (app *App) protectedPins(sessions []claude.Session) (map[string]bool, error) {
	set := map[string]bool{}
	for _, a := range app.selected() {
		ids, err := a.Jobs.Pins()
		if err != nil {
			return nil, fmt.Errorf("cannot read pins, so cannot tell which sessions to protect: %w", err)
		}
		if err := app.refuse(app.inAccount(a, "pins.json"), compat.CheckPins(ids)); err != nil {
			return nil, err
		}
		if !a.Jobs.Exists() && slices.ContainsFunc(sessions, func(s claude.Session) bool { return s.JobID != "" && app.accountOf(s) == a }) {
			missing := fmt.Sprintf("the jobs directory %s does not exist, though claude agents lists background sessions, "+
				"so claudes cannot see which are pinned. CLAUDE_CONFIG_DIR may point to the wrong directory", a.Jobs.Dir)
			if !app.force {
				return nil, refusal(errors.New(missing + ". Pass --force to go ahead anyway"))
			}
			app.warn("claudes: going ahead with --force, although %s", missing)
		}
		for _, id := range ids {
			set[id] = true
		}
	}
	return set, nil
}

// inAccount is what, naming a's account when there is more than one.
func (app *App) inAccount(a *account, what string) string {
	if !app.multi() {
		return what
	}
	return what + " of account " + a.Name
}

// refuse turns problems found in what claudes read into an error, or into a
// warning with --force.
func (app *App) refuse(source string, problems []string) error {
	if len(problems) == 0 {
		return nil
	}
	list := "\n  " + strings.Join(problems, "\n  ")
	if app.force {
		app.warn("claudes: going ahead with --force, although %s looks unfamiliar:%s", source, list)
		return nil
	}
	return refusal(fmt.Errorf("%s holds something claudes does not recognize, so it changed nothing:%s\n"+
		"Claude Code may have changed. Run claudes doctor, or pass --force to go ahead anyway", source, list))
}

func (app *App) warnVersion(ctx context.Context) {
	if app.versionChecked {
		return
	}
	app.versionChecked = true
	installed, err := app.installedVersion(ctx)
	if err != nil {
		return
	}
	if w := compat.Tested.CheckVersion(installed); w != "" {
		app.warn("claudes: %s. Run claudes doctor if anything goes wrong.", w)
	}
}

// sessionVersion is the Claude Code version a session runs: the one its
// process's executable names, or else the job's cliVersion, which Claude Code
// updates only once a respawned session takes a turn.
func (app *App) sessionVersion(s claude.Session) string {
	if s.PID > 0 && app.RunningVersion != nil {
		if v := app.RunningVersion(s.PID); v != "" {
			return v
		}
	}
	return app.accountOf(s).Jobs.Version(s.JobID)
}

// installedVersion is `claude --version`, cached in CacheDir.
func (app *App) installedVersion(ctx context.Context) (string, error) {
	binary := app.claudePath()
	return compat.CachedVersion(app.CacheDir, binary, app.Now(), func() (string, error) {
		return app.Agents.Version(ctx)
	})
}

// claudePath is where claude is on PATH, or "" when it is not found.
func (app *App) claudePath() string {
	if app.Which == nil {
		return ""
	}
	path, err := app.Which("claude")
	if err != nil {
		return ""
	}
	return path
}
