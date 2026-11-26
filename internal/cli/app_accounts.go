package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/accounts"
	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/cswap"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/teams"
)

// account is one Claude Code directory claudes manages, with the stores it
// reads there. Each account keeps its own jobs, pins, teams, and MCP
// settings.
type account struct {
	accounts.Account
	Jobs  jobs.Store
	Teams teams.Store
	MCP   mcp.Store
}

func newAccount(a accounts.Account) *account {
	return &account{Account: a, Jobs: jobs.StoreIn(a.Dir), Teams: teams.StoreIn(a.Dir), MCP: mcp.StoreIn(a.Dir, a.ConfigFile)}
}

// findAccounts lists the accounts, with cswap's profiles when cswap is on PATH
// and config.yaml does not turn it off.
func (app *App) findAccounts() {
	home, _ := os.UserHomeDir()
	o := accounts.Options{Home: home, Env: os.Getenv("CLAUDE_CONFIG_DIR"), Dirs: app.Config.AccountDirs(home)}
	if !app.Config.Accounts.Cswap.Off() {
		if _, err := app.Which("cswap"); err == nil {
			o.CswapDir, _ = cswap.Dir()
		}
	}
	app.Found = accounts.Find(o)
	app.Accounts = make([]*account, len(app.Found.Accounts))
	for i, a := range app.Found.Accounts {
		app.Accounts[i] = newAccount(a)
	}
	app.Transcripts.Projects, app.Transcripts.Homes = app.otherDirs(app.Accounts[1:], app.Accounts[1:])
}

// otherDirs are the projects directories of listed, and the Claude Code
// directories of cleaned, for transcripts.Store's Projects and Homes.
func (app *App) otherDirs(listed, cleaned []*account) (projects, homes []string) {
	for _, a := range listed {
		projects = append(projects, filepath.Join(a.Dir, "projects"))
	}
	for _, a := range cleaned {
		homes = append(homes, a.Dir)
	}
	return projects, homes
}

// multi reports whether claudes manages more than one account, so its output
// names each session's account.
func (app *App) multi() bool { return len(app.Accounts) > 1 }

// selected are the accounts a command covers: those --account names, or all.
func (app *App) selected() []*account {
	if app.only != nil {
		return app.only
	}
	return app.Accounts
}

// chooseAccounts narrows the command to the accounts --account names, and
// warns about any part of the accounts claudes cannot read.
func (app *App) chooseAccounts(cmd *cobra.Command) error {
	if cmd.Flags().Lookup("account") == nil {
		return nil
	}
	for _, err := range app.Found.Problems {
		app.warn("claudes: %v", err)
	}
	if app.accountFlag == "" {
		return nil
	}
	found, err := app.Found.Resolve(app.accountFlag)
	if err != nil {
		return err
	}
	app.only = nil
	for _, f := range found {
		if i := slices.IndexFunc(app.Accounts, func(a *account) bool { return a.Dir == f.Dir }); i >= 0 {
			app.only = append(app.only, app.Accounts[i])
		}
	}
	if len(app.only) == 0 {
		return fmt.Errorf("no account %q", app.accountFlag)
	}
	// Past sessions are read from the chosen accounts, and a removed
	// session's files from every account, since each keeps its own.
	first := app.only[0]
	var rest []*account
	for _, a := range app.Accounts {
		if a != first {
			rest = append(rest, a)
		}
	}
	app.Transcripts.Dir, app.Transcripts.Home = filepath.Join(first.Dir, "projects"), first.Dir
	app.Transcripts.Projects, app.Transcripts.Homes = app.otherDirs(app.only[1:], rest)
	return nil
}

// accountOf is the account s was listed under.
func (app *App) accountOf(s claude.Session) *account { return app.accountAt(s.ConfigDir) }

// accountAt is the account claude reaches with dir, or the one claudes runs
// as when none matches.
func (app *App) accountAt(dir claude.ConfigDir) *account {
	if i := slices.IndexFunc(app.Accounts, func(a *account) bool { return a.ConfigDir == dir }); i >= 0 {
		return app.Accounts[i]
	}
	return app.Accounts[0]
}

// resumeAccount is the account a past session resumes under: the first one
// --account names, or else the one claudes runs as.
func (app *App) resumeAccount() *account { return app.selected()[0] }

// listIn lists the sessions of each of accts; all adds finished background
// sessions. With several accounts, an error names the account.
func (app *App) listIn(ctx context.Context, accts []*account, all bool) ([]claude.Session, error) {
	var out []claude.Session
	for _, a := range accts {
		sessions, err := app.Agents.Sessions(ctx, a.ConfigDir, all)
		if err != nil {
			if app.multi() {
				return nil, fmt.Errorf("account %s: %w", a.Name, err)
			}
			return nil, err
		}
		out = append(out, sessions...)
	}
	return out, nil
}

// listOwners lists again the accounts that sessions belong to, so a wait on
// some sessions does not list every account each time.
func (app *App) listOwners(ctx context.Context, sessions ...claude.Session) ([]claude.Session, error) {
	var owners []*account
	for _, s := range sessions {
		if a := app.accountOf(s); !slices.Contains(owners, a) {
			owners = append(owners, a)
		}
	}
	return app.listIn(ctx, owners, true)
}

// teammates names the teammates at work for the lead sessionID, in whichever
// account holds its team.
func (app *App) teammates(sessionID string) []string {
	for _, a := range app.Accounts {
		if names := a.Teams.Teammates(sessionID); len(names) > 0 {
			return names
		}
	}
	return nil
}

// accountName is the name of s's account, or "" with one account, so output
// stays as it was without accounts.
func (app *App) accountName(s claude.Session) string {
	if !app.multi() {
		return ""
	}
	return app.accountOf(s).Name
}

// accountNote is a plan row's note naming s's account, or "" with one account.
func (app *App) accountNote(s claude.Session) string {
	if name := app.accountName(s); name != "" {
		return "account " + name
	}
	return ""
}

// parkEntry is s as the park file records it, with its account when there
// is more than one.
func (app *App) parkEntry(s claude.Session, wasBusy bool, pausedAt string) park.Entry {
	e := park.FromSession(s, wasBusy, pausedAt)
	if app.multi() {
		a := app.accountOf(s)
		e.Account, e.AccountDir = a.Name, a.Dir
	}
	return e
}

// parkedConfigDir is how claude reaches the account e was paused under, or
// the account claudes runs as for an entry that names none.
func (app *App) parkedConfigDir(e park.Entry) claude.ConfigDir {
	if e.AccountDir == "" {
		return claude.ConfigDir{}
	}
	return app.Found.ConfigDirOf(e.AccountDir)
}

// parkedSelected reports whether e belongs to an account the command covers.
// An entry with no account belongs to the one claudes runs as.
func (app *App) parkedSelected(e park.Entry) bool {
	if app.only == nil {
		return true
	}
	dir := app.parkedConfigDir(e)
	return slices.ContainsFunc(app.only, func(a *account) bool { return a.ConfigDir == dir })
}
