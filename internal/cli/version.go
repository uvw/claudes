package cli

import (
	"context"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/compat"
)

// sessionVersion is the Claude Code version a session runs: the one its
// process's executable names, or else the job's cliVersion, which Claude Code
// updates only once a respawned session takes a turn.
func (app *App) sessionVersion(s claude.Session) string {
	if s.PID > 0 && app.RunningVersion != nil {
		if v := app.RunningVersion(s.PID); v != "" {
			return v
		}
	}
	return app.Jobs.Version(s.JobID)
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
