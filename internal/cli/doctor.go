package cli

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/compat"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/cswap"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
)

// check is one line of claudes doctor.
type check struct {
	Name   string `json:"name" yaml:"name"`
	Result string `json:"result" yaml:"result"`
	Detail string `json:"detail" yaml:"detail"`
}

const (
	resultOK   = "ok"
	resultWarn = "warn"
	resultFail = "fail"
)

func (app *App) doctorCommand() *cobra.Command {
	var cliOnly, strict bool
	var out render.Format
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that Claude Code still looks the way claudes expects",
		Long: `Check claude's version against the range claudes was tested with, its command
line for the commands and flags claudes calls, claude agents --json and
pins.json for values claudes does not recognize, and claudes' own files.
Exits 1 when a check fails.

With more than one account, it lists each and where claudes found it, and
checks each one's sessions, pins, and teams. When cswap is on PATH, it runs
cswap list --json to check that its accounts match what claudes read from its
files, and warns about a profile claudes skips that still runs background
sessions.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := app.diagnose(cmd.Context(), cliOnly)
			failed := 0
			for _, c := range checks {
				if c.Result == resultFail || strict && c.Result == resultWarn {
					failed++
				}
			}
			if out != render.Text {
				if err := out.Encode(app.Out, checks); err != nil {
					return err
				}
			} else {
				table := [][]string{{"RESULT", "CHECK", "DETAIL"}}
				for _, c := range checks {
					lines := strings.Split(c.Detail, "\n")
					table = append(table, []string{app.result(c.Result), c.Name, lines[0]})
					for _, line := range lines[1:] {
						table = append(table, []string{"", "", line})
					}
				}
				if err := app.writeTable(app.Out, table, tableFit{}); err != nil {
					return err
				}
			}
			if failed > 0 {
				app.warn("")
				app.errorLine("%d check(s) failed.", failed)
				return errReported
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&cliOnly, "cli-only", false, "check only claude's version and command line, without listing sessions")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail on warnings too")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	cmd.AddCommand(app.doctorEnvCommand())
	return cmd
}

func (app *App) diagnose(ctx context.Context, cliOnly bool) []check {
	checks := []check{app.checkVersion(ctx), app.checkCommandLine(ctx)}
	if cliOnly {
		return checks
	}
	if app.showsAccounts() {
		checks = append(checks, app.checkAccounts())
	}
	if c, ok := app.checkCswap(ctx); ok {
		checks = append(checks, c)
	}
	for _, a := range app.Accounts {
		checks = append(checks, app.checkSessions(ctx, a), app.checkPins(a), app.checkPinsLock(a))
	}
	checks = append(checks, app.checkTranscripts(), app.checkConfig(), app.checkPark())
	for _, a := range app.Accounts {
		checks = append(checks, app.checkTeams(a))
	}
	return checks
}

// showsAccounts reports whether doctor lists the accounts: when there is
// more than one, or claudes could not read part of them.
func (app *App) showsAccounts() bool { return app.multi() || len(app.Found.Problems) > 0 }

// forAccount is a check's name, followed by a's label when there is more
// than one account.
func (app *App) forAccount(name string, a *account) string {
	if !app.multi() {
		return name
	}
	return name + " (" + app.accountLabel(a) + ")"
}

// accountLabel tells a apart from every other account: its name, and its
// directory's last element when another account shares the name, as
// ~/.claude and the cswap profile of the account it is logged in as do.
func (app *App) accountLabel(a *account) string {
	if slices.ContainsFunc(app.Accounts, func(x *account) bool { return x != a && x.Name == a.Name }) {
		return a.Name + ", " + filepath.Base(a.Dir)
	}
	return a.Name
}

func (app *App) checkAccounts() check {
	const name = "accounts"
	var lines []string
	for _, a := range app.Accounts {
		lines = append(lines, fmt.Sprintf("%s: %s, from %s", a.Name, a.Dir, strings.Join(a.From, " and ")))
	}
	if len(app.Found.Problems) > 0 {
		for _, err := range app.Found.Problems {
			lines = append(lines, err.Error())
		}
		return check{name, resultWarn, strings.Join(lines, "\n")}
	}
	return check{name, resultOK, strings.Join(lines, "\n")}
}

// checkCswap compares cswap's own list of accounts with what claudes read
// from its files, and looks for profiles claudes skips that still run
// background sessions. ok is false when claudes leaves cswap out.
func (app *App) checkCswap(ctx context.Context) (c check, ok bool) {
	const name = "cswap"
	if app.Config.Accounts.Cswap.Off() || app.Which == nil {
		return check{}, false
	}
	binary, err := app.Which("cswap")
	if err != nil {
		return check{}, false
	}
	var warnings []string
	switch out, err := app.CswapList(ctx, binary); {
	case err != nil:
		if _, _, perr := cswap.ParseList(out); perr != nil && len(out) > 0 {
			err = perr
		}
		warnings = append(warnings, fmt.Sprintf("cannot compare cswap's accounts with sequence.json: %v", err))
	default:
		active, listed, err := cswap.ParseList(out)
		if err != nil {
			warnings = append(warnings, err.Error())
			break
		}
		for _, d := range cswap.Differences(app.Found.Sequence, active, listed) {
			warnings = append(warnings, d+": claudes reads sequence.json, so it may miss an account until cswap writes it")
		}
	}
	for _, p := range app.Found.Skipped {
		if !p.Jobs {
			continue
		}
		sessions, err := app.Agents.Sessions(ctx, claude.ConfigDir{Path: p.Dir}, false)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("claudes skips %s, since %s, and cannot tell whether it runs background sessions: %v", p.Dir, p.Skipped, err))
			continue
		}
		running := 0
		for _, s := range sessions {
			if s.Running() && !s.Interactive() {
				running++
			}
		}
		if running > 0 {
			warnings = append(warnings, fmt.Sprintf("claudes skips %s, since %s, but it runs %d background session(s). "+
				"To manage them, add it to config.yaml under accounts.dirs, such as: %s: %s",
				p.Dir, p.Skipped, running, profileAccountName(p.Dir), p.Dir))
		}
	}
	if len(warnings) > 0 {
		return check{name, resultWarn, strings.Join(warnings, "\n")}, true
	}
	return check{name, resultOK, fmt.Sprintf("%d account(s), the same as cswap list --json gives", len(app.Found.Sequence.Accounts))}, true
}

// profileAccountName is a name config.yaml takes for the profile at dir, from
// the email part of the profile's name.
func profileAccountName(dir string) string {
	_, rest, _ := strings.Cut(filepath.Base(dir), "-")
	rest, _, _ = strings.Cut(rest, "_")
	name := strings.Map(func(r rune) rune {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '-' {
			return r
		}
		return -1
	}, rest)
	if name == "" || strings.Trim(name, "0123456789") == "" || name[0] == '-' {
		return "profile"
	}
	return name
}

// result colors a check's result: ok green, warn yellow, and fail red.
func (app *App) result(r string) string {
	switch r {
	case resultOK:
		return app.colors.Green(r)
	case resultWarn:
		return app.colors.Yellow(r)
	case resultFail:
		return app.colors.Red(r)
	}
	return r
}

func (app *App) checkVersion(ctx context.Context) check {
	const name = "claude"
	installed, err := app.Agents.Version(ctx)
	if err != nil {
		return check{name, resultFail, err.Error()}
	}
	where := cmp.Or(app.claudePath(), "not found on PATH")
	if w := compat.Tested.CheckVersion(installed); w != "" {
		return check{name, resultWarn, w + ", at " + where}
	}
	return check{name, resultOK, fmt.Sprintf("%s at %s, tested with %s", installed, where, compat.Tested)}
}

func (app *App) checkCommandLine(ctx context.Context) check {
	const name = "command line"
	root, err := app.Agents.Help(ctx, "")
	var agents string
	if err == nil {
		agents, err = app.Agents.Help(ctx, "agents")
	}
	if err != nil {
		return check{name, resultFail, err.Error()}
	}
	if missing := compat.CheckHelp(root, agents); len(missing) > 0 {
		return check{name, resultFail, "missing from claude's help: " + strings.Join(missing, ", ")}
	}
	return check{name, resultOK, "every command and flag claudes calls is there"}
}

func (app *App) checkSessions(ctx context.Context, a *account) check {
	name := app.forAccount("agents --json", a)
	sessions, err := app.Agents.Sessions(ctx, a.ConfigDir, true)
	if err != nil {
		return check{name, resultFail, err.Error()}
	}
	if problems := compat.CheckSessions(sessions); len(problems) > 0 {
		return check{name, resultFail, strings.Join(problems, "\n")}
	}
	background, known := 0, 0
	for _, s := range sessions {
		if s.JobID != "" {
			background++
			if a.Jobs.Version(s.JobID) != "" {
				known++
			}
		}
	}
	if background > 0 && known == 0 {
		return check{name, resultWarn, fmt.Sprintf("no background session's Claude Code version is readable in %s, "+
			"so ls shows no VERSION and restart --outdated picks nothing", a.Jobs.Dir)}
	}
	return check{name, resultOK, fmt.Sprintf("%d session(s), every kind and status known", len(sessions))}
}

func (app *App) checkPins(a *account) check {
	name := app.forAccount("pins.json", a)
	ids, err := a.Jobs.Pins()
	if err != nil {
		return check{name, resultFail, err.Error()}
	}
	if problems := compat.CheckPins(ids); len(problems) > 0 {
		return check{name, resultFail, strings.Join(problems, "\n")}
	}
	return check{name, resultOK, fmt.Sprintf("%d pinned", len(ids))}
}

// checkPinsLock warns when Claude Code may write pins.json without the lock
// claudes takes, so a pin from agent view and one from claudes at the same
// moment could lose one of them.
func (app *App) checkPinsLock(a *account) check {
	name := app.forAccount("pins lock", a)
	if a.MCP.ConfigFile == "" {
		return check{name, resultWarn, "cannot find Claude Code's settings file, so cannot tell how it writes pins.json"}
	}
	on, found, err := claude.Feature(a.MCP.ConfigFile, compat.PinsFlag)
	switch {
	case err != nil:
		return check{name, resultWarn, fmt.Sprintf("cannot read %s, so cannot tell how Claude Code writes pins.json: %v", a.MCP.ConfigFile, err)}
	case on:
		return check{name, resultWarn, compat.PinsFlag + " is on, so Claude Code may write pins.json without pins.json.lock: " +
			"pinning from agent view and from claudes at the same moment could lose one of the pins"}
	case found:
		return check{name, resultOK, "Claude Code takes pins.json.lock, as claudes does (" + compat.PinsFlag + " is off)"}
	}
	return check{name, resultOK, "Claude Code takes pins.json.lock, as claudes does (" + compat.PinsFlag + " is not set)"}
}

func (app *App) checkTranscripts() check {
	const name = "transcripts"
	list, err := app.Transcripts.List()
	switch {
	case err != nil:
		return check{name, resultFail, err.Error()}
	case !transcripts.FolderNamesMatch(list):
		return check{name, resultFail, "recent sessions live in project directories claudes would not name that way, so claudes session mv would copy to the wrong one"}
	}
	where := app.Transcripts.Dir
	if n := len(app.Transcripts.Projects); n > 0 {
		where += fmt.Sprintf(" and %d more account(s)' projects", n)
	}
	return check{name, resultOK, fmt.Sprintf("%d session(s) in %s", len(list), where)}
}

func (app *App) checkTeams(a *account) check {
	name := app.forAccount("agent teams", a)
	bad, err := a.Teams.Unreadable()
	switch {
	case err != nil:
		return check{name, resultFail, err.Error()}
	case len(bad) > 0:
		return check{name, resultWarn, fmt.Sprintf("cannot read the config.json of %s in %s, so their leads count as having teammates "+
			"and bulk commands leave them out", strings.Join(bad, ", "), a.Teams.Teams)}
	}
	return check{name, resultOK, a.Teams.Teams}
}

func (app *App) checkConfig() check {
	const name = "config.yaml"
	switch {
	case app.configErr != nil:
		return check{name, resultFail, app.configErr.Error()}
	case !config.Exists(app.ConfigPath):
		return check{name, resultOK, "none at " + app.ConfigPath + ", so the built-in defaults apply"}
	}
	return check{name, resultOK, app.ConfigPath}
}

func (app *App) checkPark() check {
	const name = "park file"
	entries, err := app.Park.Load()
	if err != nil {
		return check{name, resultFail, err.Error()}
	}
	return check{name, resultOK, fmt.Sprintf("%d parked, in %s", len(entries), app.Park.Path)}
}
