package cli

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/compat"
	"github.com/uvw/claudes/internal/config"
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
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check that Claude Code still looks the way claudes expects",
		Long: `Check claude's version against the range claudes was tested with, its command
line for the commands and flags claudes calls, claude agents --json and
pins.json for values claudes does not recognize, and claudes' own files.
Exits 1 when a check fails.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			checks := app.diagnose(cmd.Context(), cliOnly)
			failed := 0
			for _, c := range checks {
				if c.Result == resultFail || strict && c.Result == resultWarn {
					failed++
				}
			}
			if out.format != "text" {
				if err := out.encode(app.Out, checks); err != nil {
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
	out.add(cmd, app.Config.Output.Format)
	cmd.AddCommand(app.doctorEnvCommand())
	return cmd
}

func (app *App) diagnose(ctx context.Context, cliOnly bool) []check {
	checks := []check{app.checkVersion(ctx), app.checkCommandLine(ctx)}
	if cliOnly {
		return checks
	}
	return append(checks, app.checkSessions(ctx), app.checkPins(), app.checkPinsLock(), app.checkTranscripts(), app.checkConfig(), app.checkPark(), app.checkTeams())
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

func checkOK(name, detail string) check   { return check{name, resultOK, detail} }
func checkWarn(name, detail string) check { return check{name, resultWarn, detail} }
func checkFail(name, detail string) check { return check{name, resultFail, detail} }

func (app *App) checkVersion(ctx context.Context) check {
	const name = "claude"
	installed, err := app.Agents.Version(ctx)
	if err != nil {
		return checkFail(name, err.Error())
	}
	where := cmp.Or(app.claudePath(), "not found on PATH")
	if w := compat.Tested.CheckVersion(installed); w != "" {
		return checkWarn(name, w+", at "+where)
	}
	return checkOK(name, fmt.Sprintf("%s at %s, tested with %s", installed, where, compat.Tested))
}

func (app *App) checkCommandLine(ctx context.Context) check {
	const name = "command line"
	root, err := app.Agents.Help(ctx, "")
	var agents string
	if err == nil {
		agents, err = app.Agents.Help(ctx, "agents")
	}
	if err != nil {
		return checkFail(name, err.Error())
	}
	if missing := compat.CheckHelp(root, agents); len(missing) > 0 {
		return checkFail(name, "missing from claude's help: "+strings.Join(missing, ", "))
	}
	return checkOK(name, "every command and flag claudes calls is there")
}

func (app *App) checkSessions(ctx context.Context) check {
	const name = "agents --json"
	sessions, err := app.Agents.Sessions(ctx, true)
	if err != nil {
		return checkFail(name, err.Error())
	}
	if problems := compat.CheckSessions(sessions); len(problems) > 0 {
		return checkFail(name, strings.Join(problems, "\n"))
	}
	background, known := 0, 0
	for _, s := range sessions {
		if s.JobID != "" {
			background++
			if app.Jobs.Version(s.JobID) != "" {
				known++
			}
		}
	}
	if background > 0 && known == 0 {
		return checkWarn(name, fmt.Sprintf("no background session's Claude Code version is readable in %s, "+
			"so ls shows no VERSION and restart --outdated picks nothing", app.Jobs.Dir))
	}
	return checkOK(name, fmt.Sprintf("%d session(s), every kind and status known", len(sessions)))
}

func (app *App) checkPins() check {
	const name = "pins.json"
	ids, err := app.Jobs.Pins()
	if err != nil {
		return checkFail(name, err.Error())
	}
	if problems := compat.CheckPins(ids); len(problems) > 0 {
		return checkFail(name, strings.Join(problems, "\n"))
	}
	return checkOK(name, fmt.Sprintf("%d pinned", len(ids)))
}

// checkPinsLock warns when Claude Code may write pins.json without the lock
// claudes takes, so a pin from agent view and one from claudes at the same
// moment could lose one of them.
func (app *App) checkPinsLock() check {
	const name = "pins lock"
	if app.MCP.ConfigFile == "" {
		return checkWarn(name, "cannot find Claude Code's settings file, so cannot tell how it writes pins.json")
	}
	on, found, err := app.MCP.Feature(compat.PinsFlag)
	switch {
	case err != nil:
		return checkWarn(name, fmt.Sprintf("cannot read %s, so cannot tell how Claude Code writes pins.json: %v", app.MCP.ConfigFile, err))
	case on:
		return checkWarn(name, compat.PinsFlag+" is on, so Claude Code may write pins.json without pins.json.lock: "+
			"pinning from agent view and from claudes at the same moment could lose one of the pins")
	case found:
		return checkOK(name, "Claude Code takes pins.json.lock, as claudes does ("+compat.PinsFlag+" is off)")
	}
	return checkOK(name, "Claude Code takes pins.json.lock, as claudes does ("+compat.PinsFlag+" is not set)")
}

func (app *App) checkTranscripts() check {
	const name = "transcripts"
	list, err := app.Transcripts.List()
	switch {
	case err != nil:
		return checkFail(name, err.Error())
	case !transcripts.FolderNamesMatch(list):
		return checkFail(name, "recent sessions live in project directories claudes would not name that way, so claudes session mv would copy to the wrong one")
	}
	return checkOK(name, fmt.Sprintf("%d session(s) in %s", len(list), app.Transcripts.Dir))
}

func (app *App) checkTeams() check {
	const name = "agent teams"
	bad, err := app.Teams.Unreadable()
	switch {
	case err != nil:
		return checkFail(name, err.Error())
	case len(bad) > 0:
		return checkWarn(name, fmt.Sprintf("cannot read the config.json of %s in %s, so their leads count as having teammates "+
			"and bulk commands leave them out", strings.Join(bad, ", "), app.Teams.Teams))
	}
	return checkOK(name, app.Teams.Teams)
}

func (app *App) checkConfig() check {
	const name = "config.yaml"
	switch {
	case app.configErr != nil:
		return checkFail(name, app.configErr.Error())
	case !config.Exists(app.ConfigPath):
		return checkOK(name, "none at "+app.ConfigPath+", so the built-in defaults apply")
	}
	return checkOK(name, app.ConfigPath)
}

func (app *App) checkPark() check {
	const name = "park file"
	entries, err := app.Park.Load()
	if err != nil {
		return checkFail(name, err.Error())
	}
	return checkOK(name, fmt.Sprintf("%d parked, in %s", len(entries), app.Park.Path))
}
