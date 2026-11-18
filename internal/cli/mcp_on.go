package cli

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/target"
)

// The results of one server in mcp on and off.
const (
	setDone      = "done"
	setAlready   = "already"
	setPlanned   = "planned"
	setQueued    = "queued"
	setRefused   = "refused"
	setError     = "error"
	setTimedOut  = "timed out"
	setUnchanged = "unchanged"
)

// The ways mcp on and off reach a session's servers.
const (
	viaMod     = "mod"
	viaRestart = "restart"
	viaFile    = "file"
)

const (
	// modWait is the default of --timeout: how long claudes waits for the mod
	// to answer, which it does once the session is idle.
	modWait = 2 * time.Minute
	// settleWait bounds the wait for ~/.claude.json to show a change the mod
	// reports done.
	settleWait = 3 * time.Second
)

// mcpChange is one server that mcp on or off acts on.
type mcpChange struct {
	Name string `json:"name"`
	// From is on or off, as disabledMcpServers has it before, and To is the
	// state asked for.
	From   string `json:"from"`
	To     string `json:"to"`
	Result string `json:"result"`
	Note   string `json:"note"`
	// request is what was sent to the mod for it.
	request mod.Request
}

// mcpSet is what mcp on and off print with -o json or yaml.
type mcpSet struct {
	Command string `json:"command"`
	Dir     string `json:"dir"`
	// ID and Name are the session's, or "" with --in.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Via is mod, restart, or file: through the session's claudes mod, by
	// restarting the session, or by writing ~/.claude.json alone, for --in.
	Via     string      `json:"via"`
	DryRun  bool        `json:"dryRun"`
	Servers []mcpChange `json:"servers"`
}

// mcpSetFlags are the flags mcp on and off share.
type mcpSetFlags struct {
	in            string
	restart       bool
	dryRun        bool
	interactive   bool
	interactiveBG bool
	timeout       time.Duration
	out           outputFlag
}

func (f *mcpSetFlags) add(app *App, cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.in, "in", "i", "", "change this directory's list, with no session running there")
	cmd.Flags().BoolVar(&f.restart, "restart", false, "write the list and restart the session, instead of asking its claudes mod")
	cmd.Flags().BoolVar(&f.interactive, "interactive", false, "with --restart, stop an interactive session and print its resume command")
	cmd.Flags().BoolVar(&f.interactiveBG, "interactive-bg", false, "with --restart, resume an interactive session in the background")
	cmd.MarkFlagsMutuallyExclusive("interactive", "interactive-bg")
	cmd.MarkFlagsMutuallyExclusive("in", "restart")
	cmd.Flags().DurationVar(&f.timeout, "timeout", modWait, "how long to wait for the session's claudes mod to answer")
	addDryRun(cmd, &f.dryRun)
	f.out.add(cmd, app.Config.Output.Format)
}

// splitTarget takes TARGET off the end of args, unless --in names the
// directory instead.
func (f *mcpSetFlags) splitTarget(args []string) (patterns []string, tok string, err error) {
	if f.in != "" {
		return args, "", nil
	}
	if len(args) == 0 {
		return nil, "", errors.New("name the session last, or pass --in DIR")
	}
	return args[:len(args)-1], args[len(args)-1], nil
}

// mcpSetCommand is mcp on or off.
func (app *App) mcpSetCommand(verb string) *cobra.Command {
	var f mcpSetFlags
	var all bool
	to := mcp.On
	if verb == "off" {
		to = mcp.Off
	}
	cmd := &cobra.Command{
		Use:   verb + " [PATTERN...] TARGET",
		Short: "Turn MCP servers " + verb + " in a running session, without restarting it",
		Long: `Turn the MCP servers that match PATTERN ` + verb + ` in TARGET, a running session, as
/mcp ` + slashVerb(to) + ` would. A pattern is a server name, or a glob such as 'sql-*';
--all takes every server mcp ls lists for the session's directory.

claudes asks the session's claudes mod to run /mcp, which it does once the
session is idle (claudes mod install adds the mod). It waits up to --timeout
for the mod, then reads disabledMcpServers in ~/.claude.json to check that each
change took. A server already ` + verb + ` is left alone.

--restart writes the directory's disabledMcpServers instead and restarts the
session: it stops it, writes the list, and resumes it, so a turn in progress is
cut short. --in DIR writes the list of a directory where no session runs.`,
		ValidArgsFunction: app.completeSessions(runningSession, false),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			return app.setMCP(cmd.Context(), "mcp "+verb, &f, tok, func(servers []mcp.Server) (map[string]string, error) {
				names := make([]string, len(servers))
				for i, s := range servers {
					names[i] = s.Name
				}
				if all {
					return wantAll(names, to), nil
				}
				picked, err := matchServers(patterns, names)
				return wantAll(picked, to), err
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every server the session's directory has")
	f.add(app, cmd)
	return setCommand(cmd)
}

// setCommand marks mcp on or off to run alone, as a command that
// changes sessions does, and to print its own -o json or yaml.
func setCommand(cmd *cobra.Command) *cobra.Command {
	alone(cmd).Annotations[ownOutput] = "true"
	return cmd
}

func wantAll(names []string, state string) map[string]string {
	want := make(map[string]string, len(names))
	for _, n := range names {
		want[n] = state
	}
	return want
}

// matchServers picks the names each pattern matches: a glob over whole
// names, or one exact name. A pattern that matches nothing is an error.
func matchServers(patterns, names []string) ([]string, error) {
	var out []string
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("%q is not a valid pattern: %w", p, err)
		}
		found := false
		for _, n := range names {
			if ok, _ := path.Match(p, n); ok || p == n {
				found = true
				if !slices.Contains(out, n) {
					out = append(out, n)
				}
			}
		}
		if !found {
			return nil, errors.New(noServer(p, names))
		}
	}
	return out, nil
}

func noServer(pattern string, names []string) string {
	if len(names) == 0 {
		return fmt.Sprintf("no MCP server matches %q: the directory has none", pattern)
	}
	shown := make([]string, len(names))
	for i, n := range names {
		shown[i] = shell.Printable(n)
	}
	return fmt.Sprintf("no MCP server matches %q. Servers: %s", pattern, strings.Join(shown, ", "))
}

func slashVerb(state string) string {
	if state == mcp.On {
		return "enable"
	}
	return "disable"
}

// setMCP turns the servers want names on or off, in a running session's
// directory or in --in's. want maps each server to the state it should end in.
func (app *App) setMCP(ctx context.Context, command string, f *mcpSetFlags, tok string,
	want func([]mcp.Server) (map[string]string, error)) error {
	set := mcpSet{Command: command, DryRun: f.dryRun, Via: viaMod, Servers: []mcpChange{}}
	var s claude.Session
	switch {
	case f.in != "":
		dir, err := absDir(f.in)
		if err != nil {
			return err
		}
		if err := app.refuseRunningIn(ctx, dir); err != nil {
			return err
		}
		set.Dir, set.Via = dir, viaFile
	default:
		var err error
		if s, err = app.oneRunning(ctx, tok); err != nil {
			return err
		}
		if f.restart && s.Interactive() && !f.interactive && !f.interactiveBG {
			return refused(fmt.Errorf("%s is an interactive session: add --interactive or --interactive-bg to restart it", s.Label()))
		}
		set.Dir, set.ID, set.Name = s.Cwd, s.ID(), s.Label()
		if f.restart {
			set.Via = viaRestart
		}
	}
	servers, problems := app.MCP.Servers(set.Dir)
	for _, err := range problems {
		app.warn("claudes: cannot read every MCP setting: %v", err)
	}
	wants, err := want(servers)
	if err != nil {
		return err
	}
	disabled, err := app.MCP.Disabled(set.Dir)
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(wants) {
		c := mcpChange{Name: name, From: stateIn(disabled, name), To: wants[name]}
		if c.From == c.To {
			c.Result, c.Note = setAlready, "already "+c.To
		}
		set.Servers = append(set.Servers, c)
	}
	pending := slices.ContainsFunc(set.Servers, func(c mcpChange) bool { return c.Result == "" })
	var actErr error
	switch {
	case !pending:
	case f.dryRun:
		mark(&set, setPlanned, "")
	case set.Via == viaFile:
		actErr = app.writeDisabled(&set)
	case set.Via == viaRestart:
		return app.restartForMCP(ctx, &set, s, f)
	default:
		actErr = app.askMod(ctx, &set, s, f.timeout)
	}
	if err := app.printSet(set, &f.out); err != nil {
		return err
	}
	return cmpErr(actErr, set)
}

// cmpErr is actErr, or errSomeFailed when a server did not end as asked.
func cmpErr(actErr error, set mcpSet) error {
	if actErr != nil {
		return actErr
	}
	for _, c := range set.Servers {
		switch c.Result {
		case setRefused, setError, setTimedOut, setUnchanged:
			return errSomeFailed
		}
	}
	return nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// stateIn is a server's state as disabledMcpServers has it.
func stateIn(disabled []string, name string) string {
	if slices.Contains(disabled, name) {
		return mcp.Off
	}
	return mcp.On
}

// mark gives every server still to change result and note.
func mark(set *mcpSet, result, note string) {
	for i := range set.Servers {
		if set.Servers[i].Result == "" {
			set.Servers[i].Result, set.Servers[i].Note = result, note
		}
	}
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
	if err := requireRunning(picked); err != nil {
		return claude.Session{}, err
	}
	return picked[0], nil
}

// refuseRunningIn refuses --in for a directory a running session shares
// MCP settings with. That session would keep the servers it loaded, so the
// change would not reach it.
func (app *App) refuseRunningIn(ctx context.Context, dir string) error {
	key := app.MCP.ProjectKey(dir)
	if key == "" {
		return nil
	}
	sessions, err := app.listed(ctx, false)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Running() && app.MCP.ProjectKey(s.Cwd) == key {
			return refused(fmt.Errorf("%s runs in %s, and would not see the change: name it instead of --in", s.Label(), homeShort(s.Cwd)))
		}
	}
	return nil
}

// apply is what disabledMcpServers becomes with set's pending changes.
func (set *mcpSet) apply(current []string) []string {
	out := slices.Clone(current)
	for _, c := range set.Servers {
		if c.Result == setAlready {
			continue
		}
		switch {
		case c.To == mcp.Off && !slices.Contains(out, c.Name):
			out = append(out, c.Name)
		case c.To == mcp.On:
			out = slices.DeleteFunc(out, func(n string) bool { return n == c.Name })
		}
	}
	return out
}

// verify reads disabledMcpServers again and marks each server the command
// acted on done, or unchanged when the list does not show it as asked.
func (app *App) verify(set *mcpSet, acted func(mcpChange) bool, note string) error {
	disabled, err := app.MCP.Disabled(set.Dir)
	if err != nil {
		return err
	}
	for i, c := range set.Servers {
		if !acted(c) {
			continue
		}
		if stateIn(disabled, c.Name) == c.To {
			set.Servers[i].Result, set.Servers[i].Note = setDone, ""
		} else {
			set.Servers[i].Result, set.Servers[i].Note = setUnchanged, note
		}
	}
	return nil
}

func pendingChange(c mcpChange) bool { return c.Result == "" }

// writeDisabled writes the directory's list, for --in.
func (app *App) writeDisabled(set *mcpSet) error {
	if _, _, err := app.MCP.SetDisabled(set.Dir, set.apply); err != nil {
		mark(set, setError, err.Error())
		return err
	}
	return app.verify(set, pendingChange, "~/.claude.json does not show the change")
}

// askMod sends one request per server to s's claudes mod, waits for its
// results, and checks ~/.claude.json for each change.
func (app *App) askMod(ctx context.Context, set *mcpSet, s claude.Session, timeout time.Duration) error {
	if err := app.needMod(ctx, s); err != nil {
		return err
	}
	var reqs []mod.Request
	for i, c := range set.Servers {
		if pendingChange(c) {
			r := mod.NewRequest("mcp", slashVerb(c.To)+" "+c.Name)
			set.Servers[i].request = r
			reqs = append(reqs, r)
		}
	}
	host := app.isHost(s)
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
		mark(set, setQueued, "runs when this turn ends: check with claudes mcp ls")
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
	ran := func(c mcpChange) bool { r, ok := got[c.request.ID]; return ok && r.Status == mod.Done }
	if slices.ContainsFunc(set.Servers, ran) {
		// Claude Code may save the list a moment after /mcp returns.
		_ = app.poll(ctx, settleWait, func() (bool, error) {
			disabled, err := app.MCP.Disabled(set.Dir)
			if err != nil {
				return false, err
			}
			return !slices.ContainsFunc(set.Servers, func(c mcpChange) bool {
				return ran(c) && stateIn(disabled, c.Name) != c.To
			}), nil
		})
	}
	for i, c := range set.Servers {
		if c.request.ID == "" {
			continue
		}
		r, ok := got[c.request.ID]
		switch {
		case !ok:
			set.Servers[i].Result = setTimedOut
			set.Servers[i].Note = fmt.Sprintf("no answer from the mod in %s: the session may be mid-turn, waiting on a prompt, or holding a draft", timeout)
		case r.Status == mod.Done:
		case r.Status == mod.Refused:
			set.Servers[i].Result, set.Servers[i].Note = setRefused, r.Reason
		default:
			set.Servers[i].Result, set.Servers[i].Note = setError, cmpOr(r.Reason, r.Status)
		}
	}
	if err := app.verify(set, ran, "the mod ran /mcp, but ~/.claude.json does not show the change"); err != nil {
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
		return refused(fmt.Errorf("the claudes mod is not installed, so claudes cannot reach %s while it runs. "+
			"Install it with claudes mod install, then type /reload-plugins in the session, or pass --restart", s.Label()))
	case !plugin.Enabled:
		return refused(fmt.Errorf("the claudes mod is turned off. Turn it on with claude plugin enable %s, "+
			"then type /reload-plugins in %s, or pass --restart", mod.ID, s.Label()))
	}
	return refused(fmt.Errorf("%s has not loaded the claudes mod: type /reload-plugins in it, or pass --restart", s.Label()))
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// restartForMCP writes the list and restarts s: it stops s, writes, and
// resumes it, so the session starts with the new list. The session running
// claudes cannot be written for after it stops, so its list is written first.
func (app *App) restartForMCP(ctx context.Context, set *mcpSet, s claude.Session, f *mcpSetFlags) error {
	write := func() error {
		_, _, err := app.MCP.SetDisabled(set.Dir, set.apply)
		return err
	}
	if app.isHost(s) {
		if err := write(); err != nil {
			return err
		}
		if err := app.verify(set, pendingChange, "~/.claude.json does not show the change"); err != nil {
			return err
		}
		if err := app.printSet(*set, &f.out); err != nil {
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
	if verr := app.verify(set, pendingChange, "~/.claude.json does not show the change"); err == nil {
		err = verr
	}
	if perr := app.printSet(*set, &f.out); perr != nil {
		return perr
	}
	if s.Interactive() && f.interactive && err == nil && f.out.format == "text" {
		printManual(app, "Resume it in its own terminal, or run claudes resume:", []claude.Session{s})
	}
	if err != nil {
		return err
	}
	return cmpErr(nil, *set)
}

func (app *App) printSet(set mcpSet, out *outputFlag) error {
	if out.format != "text" {
		return out.encode(app.Out, set)
	}
	where := homeShort(set.Dir)
	switch set.Via {
	case viaFile:
		app.say("MCP servers in %s:", where)
	case viaRestart:
		app.say("MCP servers of %s, in %s, by restarting it:", set.Name, where)
	default:
		app.say("MCP servers of %s, in %s, through its claudes mod:", set.Name, where)
	}
	table := [][]string{{"NAME", "RESULT", "NOTE"}}
	for _, c := range set.Servers {
		table = append(table, []string{shell.Printable(c.Name), app.setResult(c), c.Note})
	}
	if err := style.Table(app.Out, table); err != nil {
		return err
	}
	if set.DryRun {
		app.say("\nDry run: nothing was changed.")
	}
	return nil
}

// setResult is a server's result, with the state it is now in for one that
// changed, colored like a session's progress.
func (app *App) setResult(c mcpChange) string {
	switch c.Result {
	case setDone:
		return app.colors.Green("turned " + c.To)
	case setPlanned, setQueued:
		return c.Result + " " + c.To
	case setAlready:
		return app.colors.Faint(c.Result)
	}
	return app.colors.Red(c.Result)
}
