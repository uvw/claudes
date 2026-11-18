package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/selection"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/target"
)

const (
	// modWait is the default of --timeout: how long claudes waits for the mod
	// to answer, which it does once the session is idle.
	modWait = 2 * time.Minute
	// settleWait bounds the wait for ~/.claude.json to show a change the mod
	// reports done.
	settleWait = 3 * time.Second
)

// switchFlags are the flags of mcp on, off, and use.
type switchFlags struct {
	in            string
	viaRestart    bool
	dryRun        bool
	interactive   bool
	interactiveBG bool
	timeout       time.Duration
	out           render.Format
}

// switchCommand marks cmd, which is mcp on, off, or use, to run alone, as a
// command that changes sessions does, and to print its own -o json or yaml,
// and adds the flags they share.
func (app *App) switchCommand(cmd *cobra.Command, f *switchFlags) *cobra.Command {
	cmd.Flags().StringVarP(&f.in, "in", "i", "", "change this directory's list, with no session running there")
	cmd.Flags().BoolVar(&f.viaRestart, "restart", false, "write the list and restart the session, instead of asking its claudes mod")
	cmd.Flags().BoolVar(&f.interactive, "interactive", false, "with --restart, stop an interactive session and print its resume command")
	cmd.Flags().BoolVar(&f.interactiveBG, "interactive-bg", false, "with --restart, resume an interactive session in the background")
	cmd.MarkFlagsMutuallyExclusive("interactive", "interactive-bg")
	cmd.MarkFlagsMutuallyExclusive("in", "restart")
	cmd.Flags().DurationVar(&f.timeout, "timeout", modWait, "how long to wait for the session's claudes mod to answer")
	cmd.Flags().BoolVarP(&f.dryRun, "dry-run", "n", false, "show the plan and change nothing")
	app.addOutput(cmd, &f.out, app.Config.Output.Format)
	alone(cmd).Annotations[ownOutput] = "true"
	return cmd
}

// splitTarget takes TARGET off the end of args, unless --in names the
// directory instead.
func (f *switchFlags) splitTarget(args []string) (patterns []string, tok string, err error) {
	if f.in != "" {
		return args, "", nil
	}
	if len(args) == 0 {
		return nil, "", errors.New("name the session last, or pass --in DIR")
	}
	return args[:len(args)-1], args[len(args)-1], nil
}

// toggleCommand builds mcp on or off, which turn servers to the state to. Each
// command's own file sets RunE, so it runs from there.
func (app *App) toggleCommand(to string, f *switchFlags, all *bool) *cobra.Command {
	verb := to
	cmd := &cobra.Command{
		Use:   verb + " [PATTERN...] TARGET",
		Short: "Turn MCP servers " + verb + " in a running session, without restarting it",
		Long: `Turn the MCP servers that match PATTERN ` + verb + ` in TARGET, a running session, as
/mcp ` + mcp.Verb(to) + ` would. A pattern is a server name, or a glob such as 'sql-*';
--all takes every server mcp ls lists for the session's directory.

claudes asks the session's claudes mod to run /mcp, which it does once the
session is idle (claudes mod install adds the mod). It waits up to --timeout
for the mod, then reads disabledMcpServers in ~/.claude.json to check that each
change took. A server already ` + verb + ` is left alone.

--restart writes the directory's disabledMcpServers instead and restarts the
session: it stops it, writes the list, and resumes it, so a turn in progress is
cut short. --in DIR writes the list of a directory where no session runs.`,
		ValidArgsFunction: app.completeSessions(runningSession, false),
	}
	cmd.Flags().BoolVar(all, "all", false, "every server the session's directory has")
	return app.switchCommand(cmd, f)
}

// toggle runs mcp on or off with args, the patterns and TARGET.
func (app *App) toggle(ctx context.Context, to string, f *switchFlags, all bool, args []string) error {
	patterns, tok, err := f.splitTarget(args)
	if err != nil {
		return err
	}
	if len(patterns) == 0 && !all {
		return errors.New("name a server or a pattern, or pass --all")
	}
	if len(patterns) > 0 && all {
		return errors.New("name servers or pass --all, not both")
	}
	return app.switchMCP(ctx, "mcp "+to, f, tok, func(servers []mcp.Server) (map[string]string, error) {
		names := make([]string, len(servers))
		for i, s := range servers {
			names[i] = s.Name
		}
		if all {
			return mcp.Want(names, to), nil
		}
		picked, err := mcp.Match(patterns, names)
		return mcp.Want(picked, to), err
	})
}

// switchMCP turns the servers want names on or off, in a running session's
// directory or in --in's. want maps each server to the state it should end in.
func (app *App) switchMCP(ctx context.Context, command string, f *switchFlags, tok string,
	want func([]mcp.Server) (map[string]string, error)) error {
	sw := mcp.Switch{Command: command, DryRun: f.dryRun, Via: mcp.ViaMod, Servers: []mcp.Change{}}
	var s claude.Session
	switch {
	case f.in != "":
		dir, err := paths.Real(f.in)
		if err != nil {
			return err
		}
		if err := app.refuseRunningIn(ctx, dir); err != nil {
			return err
		}
		sw.Dir, sw.Via = dir, mcp.ViaFile
	default:
		var err error
		if s, err = app.oneRunning(ctx, tok); err != nil {
			return err
		}
		if f.viaRestart && s.Interactive() && !f.interactive && !f.interactiveBG {
			return refusal(fmt.Errorf("%s is an interactive session: add --interactive or --interactive-bg to restart it", s.Label()))
		}
		sw.Dir, sw.ID, sw.Name = s.Cwd, s.ID(), s.Label()
		if f.viaRestart {
			sw.Via = mcp.ViaRestart
		}
	}
	servers, problems := app.MCP.Servers(sw.Dir)
	for _, err := range problems {
		app.warn("claudes: cannot read every MCP setting: %v", err)
	}
	wants, err := want(servers)
	if err != nil {
		return err
	}
	disabled, err := app.disabled(sw.Dir)
	if err != nil {
		return err
	}
	sw.Plan(wants, disabled)
	var actErr error
	switch {
	case !sw.Pending():
	case f.dryRun:
		sw.Mark(mcp.Planned, "")
	case sw.Via == mcp.ViaFile:
		actErr = app.writeDisabled(&sw)
	case sw.Via == mcp.ViaRestart:
		return app.restartForMCP(ctx, &sw, s, f)
	default:
		actErr = app.askMod(ctx, &sw, s, f.timeout)
	}
	if err := app.printSwitch(sw, f.out); err != nil {
		return err
	}
	if actErr == nil && sw.Failed() {
		return errSomeFailed
	}
	return actErr
}

// oneRunning is the running session tok names.
func (app *App) oneRunning(ctx context.Context, tok string) (claude.Session, error) {
	sessions, err := app.listed(ctx, true)
	if err != nil {
		return claude.Session{}, err
	}
	picked, err := target.Resolve([]string{tok}, sessions)
	if err != nil {
		return claude.Session{}, err
	}
	if err := selection.RequireRunning(picked); err != nil {
		return claude.Session{}, err
	}
	return picked[0], nil
}

func (app *App) claudeConfig() (claude.Config, error) { return claude.ReadConfig(app.MCP.ConfigFile) }

// disabled is the disabledMcpServers list of dir's entry in ~/.claude.json.
func (app *App) disabled(dir string) ([]string, error) {
	cfg, err := app.claudeConfig()
	if err != nil {
		return nil, err
	}
	return cfg.Disabled(dir), nil
}

// refuseRunningIn refuses --in for a directory a running session shares MCP
// settings with. That session would keep the servers it loaded, so the change
// would not reach it.
func (app *App) refuseRunningIn(ctx context.Context, dir string) error {
	cfg, err := app.claudeConfig()
	if err != nil {
		return err
	}
	key, ok := cfg.EntryKey(dir)
	if !ok {
		return nil
	}
	sessions, err := app.listed(ctx, false)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if other, _ := cfg.EntryKey(s.Cwd); s.Running() && other == key {
			return refusal(fmt.Errorf("%s runs in %s, and would not see the change: name it instead of --in",
				s.Label(), s.Cwd))
		}
	}
	return nil
}

// verify reads disabledMcpServers again and marks each server the command
// acted on done, or unchanged when the list does not show it as asked.
func (app *App) verify(sw *mcp.Switch, acted func(mcp.Change) bool, note string) error {
	disabled, err := app.disabled(sw.Dir)
	if err != nil {
		return err
	}
	sw.Verify(disabled, acted, note)
	return nil
}

// noChange is the note on a server claudes wrote but whose change
// ~/.claude.json does not show.
const noChange = "~/.claude.json does not show the change"

// writeDisabled writes the directory's list, for --in.
func (app *App) writeDisabled(sw *mcp.Switch) error {
	if _, _, err := claude.SetDisabled(app.MCP.ConfigFile, sw.Dir, sw.Apply); err != nil {
		sw.Mark(mcp.Failed, err.Error())
		return err
	}
	return app.verify(sw, mcp.Change.Pending, noChange)
}

// askMod sends one request per server to s's claudes mod, waits for its
// results, and checks ~/.claude.json for each change.
func (app *App) askMod(ctx context.Context, sw *mcp.Switch, s claude.Session, timeout time.Duration) error {
	if err := app.needMod(ctx, s); err != nil {
		return err
	}
	sent := map[string]mod.Request{}
	var reqs []mod.Request
	for _, c := range sw.Servers {
		if c.Pending() {
			r := mod.NewRequest("mcp", mcp.Verb(c.To)+" "+c.Name)
			sent[c.Name] = r
			reqs = append(reqs, r)
		}
	}
	host := app.host().Is(s)
	expires := app.Now().Add(timeout + 10*time.Second)
	if host {
		// The mod runs nothing while a turn runs, and the turn running claudes
		// cannot end while claudes waits.
		expires = app.Now().Add(app.waitTimeout())
	}
	if err := app.Mod.Send(s.SessionID, reqs, expires); err != nil {
		return err
	}
	if host {
		sw.Mark(mcp.Queued, "runs when this turn ends: check with claudes mcp ls")
		return nil
	}
	var got map[string]mod.Result
	waitErr := app.poll(ctx, timeout, func() (bool, error) {
		var err error
		got, err = app.Mod.Results(reqs)
		return len(got) == len(reqs), err
	})
	clearErr := app.Mod.Clear(s.SessionID, reqs)
	if waitErr != nil && !errors.Is(waitErr, errTimedOut) {
		return errors.Join(waitErr, clearErr)
	}
	ran := func(c mcp.Change) bool {
		r, ok := got[sent[c.Name].ID]
		return ok && r.Status == mod.Done
	}
	if slices.ContainsFunc(sw.Servers, ran) {
		// Claude Code may save the list a moment after /mcp returns.
		_ = app.poll(ctx, settleWait, func() (bool, error) {
			disabled, err := app.disabled(sw.Dir)
			if err != nil {
				return false, err
			}
			for _, c := range sw.Servers {
				if ran(c) && mcp.StateIn(disabled, c.Name) != c.To {
					return false, nil
				}
			}
			return true, nil
		})
	}
	for i, c := range sw.Servers {
		r, isSent := sent[c.Name]
		if !isSent {
			continue
		}
		res, ok := got[r.ID]
		switch {
		case !ok:
			sw.Servers[i].Result = mcp.TimedOut
			sw.Servers[i].Note = fmt.Sprintf("no answer from the mod in %s: the session may be mid-turn, waiting on a prompt, or holding a draft", timeout)
		case res.Status == mod.Done:
		case res.Status == mod.Refused:
			sw.Servers[i].Result, sw.Servers[i].Note = mcp.Refused, res.Reason
		default:
			sw.Servers[i].Result, sw.Servers[i].Note = mcp.Failed, cmp.Or(res.Reason, res.Status)
		}
	}
	if err := app.verify(sw, ran, "the mod ran /mcp, but "+noChange); err != nil {
		return err
	}
	return clearErr
}

// needMod refuses a session that has not loaded the claudes mod, and says
// what would load it. A mod the session loaded counts however it got there,
// such as with --plugin-dir.
func (app *App) needMod(ctx context.Context, s claude.Session) error {
	if app.modLoaded(s) != "" {
		return nil
	}
	plugin, installed, err := app.modPlugin(ctx)
	switch {
	case err != nil:
		return err
	case !installed:
		return refusal(fmt.Errorf("the claudes mod is not installed, so claudes cannot reach %s while it runs. "+
			"Install it with claudes mod install, then type /reload-plugins in the session, or pass --restart", s.Label()))
	case !plugin.Enabled:
		return refusal(fmt.Errorf("the claudes mod is turned off. Turn it on with claude plugin enable %s, "+
			"then type /reload-plugins in %s, or pass --restart", mod.ID, s.Label()))
	}
	return refusal(fmt.Errorf("%s has not loaded the claudes mod: type /reload-plugins in it, or pass --restart", s.Label()))
}

// restartForMCP writes the list and restarts s: it stops s, writes, and
// resumes it, so the session starts with the new list. The session running
// claudes cannot be written for after it stops, so its list is written first.
func (app *App) restartForMCP(ctx context.Context, sw *mcp.Switch, s claude.Session, f *switchFlags) error {
	write := func() error {
		_, _, err := claude.SetDisabled(app.MCP.ConfigFile, sw.Dir, sw.Apply)
		return err
	}
	if app.host().Is(s) {
		if err := write(); err != nil {
			return err
		}
		if err := app.verify(sw, mcp.Change.Pending, noChange); err != nil {
			return err
		}
		if err := app.printSwitch(*sw, f.out); err != nil {
			return err
		}
		return app.restartHost(ctx, s)
	}
	hand := context.WithoutCancel(ctx)
	var err error
	switch {
	case s.Interactive() && f.interactive:
		if err = app.parkAndStop(hand, s); err == nil {
			err = write()
		}
	default:
		err = app.relaunchAround(hand, s, "", write)
	}
	if verr := app.verify(sw, mcp.Change.Pending, noChange); err == nil {
		err = verr
	}
	if perr := app.printSwitch(*sw, f.out); perr != nil {
		return perr
	}
	if s.Interactive() && f.interactive && err == nil && f.out == render.Text {
		printManual(app, "Resume it in its own terminal, or run claudes resume:", []claude.Session{s})
	}
	if err == nil && sw.Failed() {
		return errSomeFailed
	}
	return err
}

// printSwitch prints what mcp on, off, or use did, as a table or as out.
func (app *App) printSwitch(sw mcp.Switch, out render.Format) error {
	if out != render.Text {
		return out.Encode(app.Out, sw)
	}
	before, after := "MCP servers of "+shell.Printable(sw.Name)+", in ", ", through its claudes mod:"
	switch sw.Via {
	case mcp.ViaFile:
		before, after = "MCP servers in ", ":"
	case mcp.ViaRestart:
		after = ", by restarting it:"
	}
	app.say("%s%s%s", before, app.dirIn(sw.Dir, render.Width(before+after)), after)
	table := [][]string{{"NAME", "RESULT", "NOTE"}}
	for _, c := range sw.Servers {
		table = append(table, []string{shell.Printable(c.Name), app.switchResult(c), c.Note})
	}
	if err := app.writeTable(app.Out, table, tableFit{fitLast: true}); err != nil {
		return err
	}
	if sw.DryRun {
		app.dryRunDone()
	}
	return nil
}

// switchResult is a server's result, with the state it is now in for one that
// changed, colored like a session's progress.
func (app *App) switchResult(c mcp.Change) string {
	switch c.Result {
	case mcp.Done:
		return app.colors.Green("turned " + c.To)
	case mcp.Planned, mcp.Queued:
		return c.Result + " " + c.To
	case mcp.Already:
		return app.colors.Faint(c.Result)
	}
	return app.colors.Red(c.Result)
}
