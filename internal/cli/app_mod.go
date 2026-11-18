package cli

import (
	"context"
	"slices"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mod"
)

// modPlugin is the claudes mod as claude plugin list shows it, and whether it
// is installed.
func (app *App) modPlugin(ctx context.Context) (claude.Plugin, bool, error) {
	plugins, err := app.Agents.Plugins(ctx)
	if err != nil {
		return claude.Plugin{}, false, err
	}
	i := slices.IndexFunc(plugins, func(p claude.Plugin) bool { return p.ID == mod.ID })
	if i < 0 {
		return claude.Plugin{}, false, nil
	}
	return plugins[i], true, nil
}

// modMarketplace is the claudes marketplace, and whether Claude Code has it.
func (app *App) modMarketplace(ctx context.Context) (claude.Marketplace, bool, error) {
	list, err := app.Agents.Marketplaces(ctx)
	if err != nil {
		return claude.Marketplace{}, false, err
	}
	i := slices.IndexFunc(list, func(m claude.Marketplace) bool { return m.Name == mod.Marketplace })
	if i < 0 {
		return claude.Marketplace{}, false, nil
	}
	return list[i], true, nil
}

// modStatus says whether the mod is installed, and which running sessions
// loaded it.
func (app *App) modStatus(ctx context.Context) (modState, error) {
	st := modState{Dir: app.Mod.Dir, Sessions: []modSession{}}
	plugin, installed, err := app.modPlugin(ctx)
	if err != nil {
		return st, err
	}
	st.Installed, st.Enabled, st.Version, st.Scope = installed, plugin.Enabled, plugin.Version, plugin.Scope
	market, found, err := app.modMarketplace(ctx)
	if err != nil {
		return st, err
	}
	if found {
		st.Source = market.Where()
	}
	sessions, err := app.Agents.Sessions(ctx, false)
	if err != nil {
		return st, err
	}
	for _, s := range sessions {
		if s.Running() {
			st.Sessions = append(st.Sessions, modSession{ID: s.ID(), Name: s.Label(), Cwd: s.Cwd, Version: app.modLoaded(s)})
		}
	}
	return st, nil
}

// modLoaded is the mod version a running session loaded, or "" when it shows
// no sign of the mod since it started.
func (app *App) modLoaded(s claude.Session) string {
	p, ok, err := app.Mod.Presence(s.SessionID)
	if err != nil || !ok || p.Time().Before(s.Started()) {
		return ""
	}
	return p.Version
}

// modState is what claudes mod status prints with -o json or yaml.
type modState struct {
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Version   string `json:"version"`
	Scope     string `json:"scope"`
	// Source is where the claudes marketplace comes from, or "".
	Source string `json:"source"`
	// Dir is the directory claudes and the mod share.
	Dir      string       `json:"dir"`
	Sessions []modSession `json:"sessions"`
}

// modSession is a running session, and the mod version it loaded, or "".
type modSession struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Cwd     string `json:"cwd"`
	Version string `json:"version"`
}
