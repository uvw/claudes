// Package cli is the claudes command line.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/mod"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/procs"
	"github.com/uvw/claudes/internal/safefile"
	"github.com/uvw/claudes/internal/selfupdate"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/teams"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
)

// Version and Commit are set at build time with -ldflags
// "-X github.com/uvw/claudes/internal/cli.Version=...".
var (
	Version = "dev"
	Commit  = ""
)

// buildVersion prefers the ldflags values, then the module version go install
// records.
func buildVersion() string {
	if Version != "dev" {
		if Commit != "" {
			return Version + " (" + Commit + ")"
		}
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}

const (
	defaultPing  = "Claude Code was restarted while you were working. Pick up where you left off."
	pollInterval = 15 * time.Second
)

// errReported means the command already told the user what went wrong.
var errReported = errors.New("failures were reported")

// errSomeFailed means the command reported sessions it could not handle.
var errSomeFailed = fmt.Errorf("%w: some sessions failed", errReported)

// Exit codes, so a script or an agent can tell what happened without reading
// the text.
const (
	exitFailed      = 1   // anything else that went wrong
	exitNoMatch     = 2   // a target named no session, or several
	exitSomeFailed  = 3   // the command reported sessions it could not handle
	exitRefused     = 4   // a safety or compatibility check stopped it
	exitBusy        = 5   // another claudes was changing sessions
	exitInterrupted = 130 // Ctrl-C
)

// App is everything the commands touch, so tests can swap it.
type App struct {
	Agents    claude.Agents
	Jobs      jobs.Store
	Park      park.File
	Out       io.Writer
	Err       io.Writer
	In        io.Reader
	Now       func() time.Time
	Sleep     func(ctx context.Context, d time.Duration) error
	Terminate func(pid int, sessionStarted, listedAt time.Time) error
	// Started is when a process started, if it runs.
	Started func(pid int) (time.Time, bool)
	// RunningVersion is the Claude Code version a process runs, read from its
	// executable, or "" when that does not say.
	RunningVersion func(pid int) string
	// Alive reports whether the process that started at started still runs;
	// a PID a newer process took over does not count.
	Alive func(pid int, started time.Time) bool
	Exec  func(binary string, args ...string) error
	// Transcripts is ~/.claude/projects, read by claudes session.
	Transcripts transcripts.Store
	// Trash moves a file or directory to the system trash.
	Trash func(path string) error
	// Teams is ~/.claude/teams and ~/.claude/tasks, where agent teams live.
	Teams teams.Store
	// ClaudeConfig is Claude Code's settings file, ~/.claude.json, which
	// claudes only reads, for the flags doctor reports.
	ClaudeConfig string
	// MCP is where Claude Code keeps MCP servers, which claudes mcp reads.
	MCP mcp.Store
	// Mod is the directory claudes and the claudes mod share.
	Mod mod.Channel
	// Chdir changes the working directory before a handoff to claude.
	Chdir func(dir string) error
	// Which finds a program on PATH, as exec.LookPath does.
	Which func(file string) (string, error)
	// HTTP downloads releases for claudes upgrade, from ReleasesURL.
	HTTP        *http.Client
	ReleasesURL string
	// Executable is the path of the running claudes, as os.Executable gives.
	Executable func() (string, error)
	// Provenance checks a downloaded archive's build provenance, and says what
	// it found; fromGitHub is whether the archive came from GitHub.
	Provenance func(ctx context.Context, archive string, fromGitHub bool) (string, error)
	// CacheDir holds the cached `claude --version`; "" turns the cache off.
	CacheDir string
	// HostID is the session running claudes, if any. It is handled last and
	// never pinged, because stopping it ends this command.
	HostID string
	// HostJob is the job id of the background session running claudes, from
	// CLAUDE_JOB_DIR: a second way to find it when HostID is not set.
	HostJob string
	// listedAt is when the sessions a command acts on were listed. A process
	// that started later only reused a session's PID.
	listedAt time.Time
	// lastList is what that listing returned.
	lastList []claude.Session
	// Config is config.yaml, read from ConfigPath. Flags always win over it.
	Config     config.Config
	ConfigPath string
	// Interrupt returns a context that the first Ctrl-C cancels, and the
	// function that restores the default, so a second Ctrl-C ends claudes.
	Interrupt func(context.Context) (context.Context, context.CancelFunc)
	// Width is how many columns the terminal Out writes to has, or 0 when Out
	// is not a terminal.
	Width func() int
	// Edit opens a file in the user's editor and waits for it to close.
	Edit func(path string) error
	// setupErr stops every claudes command before it starts, such as when the
	// home directory cannot be found. configErr is a config.yaml claudes cannot
	// use; it stops every command but claudes config, which can fix it.
	setupErr, configErr error
	// runningKnown is whether the last pastSessions call could tell which
	// sessions are running.
	runningKnown bool
	// LockPath is the lock that lets one claudes at a time change sessions;
	// "" turns it off.
	LockPath string
	// unlockRun releases that lock while this run holds it.
	unlockRun func()
	// rep collects what a command did, for -o json and -o yaml.
	rep *report
	// endsOnInterrupt marks a command that runs until Ctrl-C, such as
	// logs -f, so Ctrl-C ends it normally.
	endsOnInterrupt bool
	// logger and logLevel are the diagnostic log; see log.
	logger   *slog.Logger
	logLevel *slog.LevelVar
	// force is --force: act although a compatibility check failed.
	force bool
	// versionChecked is set once the installed version was compared with the
	// tested range, so the warning shows once per run.
	versionChecked bool
	// Environ is the environment that decides colors; nil leaves them off.
	Environ func() []string
	// colors and errColors style text for Out and Err.
	colors, errColors style.Palette
}

func newApp() *App {
	app := &App{

		Out:            os.Stdout,
		Err:            os.Stderr,
		In:             os.Stdin,
		Now:            time.Now,
		Sleep:          sleep,
		Terminate:      procs.Terminate,
		Started:        procs.Started,
		RunningVersion: procs.RunningVersion,
		Trash:          trash.Move,
		Alive:          procs.Running,
		Exec:           claude.Exec,
		Edit:           editFile,
		Interrupt: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(ctx, os.Interrupt)
		},
		Which:       exec.LookPath,
		Chdir:       os.Chdir,
		HTTP:        &http.Client{Timeout: 5 * time.Minute},
		ReleasesURL: selfupdate.Releases,
		Executable:  os.Executable,
		HostID:      os.Getenv("CLAUDE_CODE_SESSION_ID"),
		HostJob:     hostJob(os.Getenv("CLAUDE_JOB_DIR")),
		Environ:     os.Environ,
	}
	app.Provenance = app.ghProvenance
	app.Width = func() int { return style.TerminalWidth(app.Out) }
	client := claude.NewClient()
	client.Log = app.log()
	app.Agents = client
	var jobsErr, parkErr error
	app.Jobs, jobsErr = jobs.DefaultStore()
	app.Park, parkErr = park.DefaultFile()
	app.setupErr = errors.Join(jobsErr, parkErr)
	if parkErr == nil {
		app.LockPath = filepath.Join(filepath.Dir(app.Park.Path), "run.lock")
	}
	// A missing Claude Code directory is setupErr's to report, through Jobs.
	app.Transcripts, _ = transcripts.DefaultStore()
	app.Teams, _ = teams.DefaultStore()
	app.ClaudeConfig, _ = paths.ClaudeConfigFile()
	app.MCP, _ = mcp.DefaultStore()
	// Without a state directory, setupErr has already stopped every command.
	app.Mod, _ = mod.DefaultChannel()
	// Without a cache directory, claudes asks claude for its version each time.
	app.CacheDir, _ = paths.Cache()
	if app.ConfigPath, app.configErr = config.Path(); app.configErr == nil {
		app.Config, app.configErr = config.Load(app.ConfigPath)
	}
	return app
}

// waitTimeout is the default of --timeout: wait.timeout from the config, or 30
// minutes.
func (app *App) waitTimeout() time.Duration {
	if d := time.Duration(app.Config.Wait.Timeout); d > 0 {
		return d
	}
	return 30 * time.Minute
}

// pingPrompt is ping.text from the config, or builtin.
func (app *App) pingPrompt(builtin string) string {
	if app.Config.Ping.Text != "" {
		return app.Config.Ping.Text
	}
	return builtin
}

// Main runs claudes and returns its exit code.
func Main(args []string) int { return newApp().Run(args) }

// hostJob is the job id a CLAUDE_JOB_DIR names, such as ff78d735 for
// ~/.claude/jobs/ff78d735, or "" when it names none.
func hostJob(dir string) string {
	if id := filepath.Base(filepath.Clean(dir)); dir != "" && jobs.ValidID(id) {
		return id
	}
	return ""
}

// fixesConfig reports whether command still runs with a broken config.yaml:
// config and doctor help repair it, and the rest read nothing from it. A
// shell profile runs init and completion, so failing them would put the error
// in every new shell.
func fixesConfig(command string) bool {
	switch command {
	case "config", "doctor", "help", "-h", "--help", "--version",
		"init", "completion", "skill", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	}
	return false
}

// Run runs claudes with app's dependencies and returns the exit code.
func (app *App) Run(args []string) int {
	var env []string
	if app.Environ != nil {
		env = app.Environ()
	}
	app.colors, app.errColors = style.Detect(app.Out, env), style.Detect(app.Err, env)
	root := app.rootCommand()
	if passThrough(root, args) {
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if err := app.Exec("claude", append([]string{"agents"}, args...)...); err != nil {
			app.errorLine("claudes: cannot run claude agents: %v", err)
			return 1
		}
		return 0
	}
	if app.setupErr != nil {
		app.errorLine("claudes: %v", app.setupErr)
		return 1
	}
	if app.configErr != nil && !fixesConfig(commandName(args)) {
		app.errorLine("claudes: %v", app.configErr)
		fmt.Fprintln(app.Err, "Fix it with: claudes config edit")
		return 1
	}
	if name := os.Getenv("CLAUDES_LOG"); name != "" {
		if err := app.setLogLevel(name); err != nil {
			app.warn("claudes: %v", err)
		}
	}
	ctx, stop := app.Interrupt(context.Background())
	defer stop()
	// After the first Ctrl-C, commands finish the session in hand, which runs
	// under context.WithoutCancel, and report. A second one ends claudes at
	// once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	root.SetArgs(args)
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	if args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd {
		// Cobra notes each completion's directive on stderr, and not every
		// shell's completion script hides it.
		root.SetErr(io.Discard)
	}
	defer app.unlock()
	defer app.printReport()
	err := root.ExecuteContext(ctx)
	app.record(func(r *report) {
		switch {
		case ctx.Err() != nil && !app.endsOnInterrupt:
			r.Error = "interrupted"
		case err != nil && !errors.Is(err, errSomeFailed) && !errors.Is(err, errReported):
			r.Error = err.Error()
		}
	})
	var match *target.MatchError
	var refusal refusal
	switch {
	case ctx.Err() != nil && app.endsOnInterrupt && err == nil:
		return 0
	case ctx.Err() != nil:
		fmt.Fprintln(app.Err)
		app.errorLine("claudes: interrupted. Some sessions may already be stopped or restarted: check claudes ls")
		return exitInterrupted
	case err == nil:
		return 0
	case errors.Is(err, errSomeFailed):
		return exitSomeFailed
	case errors.Is(err, errReported):
		return exitFailed
	}
	app.errorLine("claudes: %s", shell.PrintableText(err.Error()))
	switch {
	case errors.Is(err, errBusy):
		return exitBusy
	case errors.As(err, &refusal):
		return exitRefused
	case errors.As(err, &match):
		return exitNoMatch
	}
	return exitFailed
}

// lockStale is how long the run lock outlives a claudes that died holding it.
const lockStale = 30 * time.Second

// errBusy means another claudes holds the run lock.
var errBusy = errors.New("another claudes is changing sessions")

// runsAlone marks a command that changes sessions or pins. One such command
// runs at a time, so two cannot stop the same session twice.
const runsAlone = "runs-alone"

func alone(cmd *cobra.Command) *cobra.Command {
	cmd.Annotations = map[string]string{runsAlone: "true"}
	return cmd
}

// lock takes the run lock for a command marked runsAlone, unless it is a dry
// run, which changes nothing.
func (app *App) lock(cmd *cobra.Command) error {
	if cmd.Annotations[runsAlone] == "" || app.LockPath == "" || app.unlockRun != nil {
		return nil
	}
	if f := cmd.Flags().Lookup("dry-run"); f != nil && f.Value.String() == "true" {
		return nil
	}
	release, err := safefile.Lock{Path: app.LockPath, Stale: lockStale, Attempts: 1}.Acquire()
	if errors.Is(err, safefile.ErrHeld) {
		return fmt.Errorf("%w, so this one changed nothing. Try again once it ends; "+
			"a claudes that crashed frees %s within %s", errBusy, app.LockPath, lockStale)
	}
	if err != nil {
		return fmt.Errorf("cannot take the run lock %s: %w", app.LockPath, err)
	}
	app.unlockRun = release
	app.log().Debug("took the run lock", "path", app.LockPath)
	return nil
}

// unlock releases the run lock, if this run holds it.
func (app *App) unlock() {
	if app.unlockRun != nil {
		app.unlockRun()
		app.unlockRun = nil
		app.log().Debug("released the run lock")
	}
}

// commandName is the first argument after claudes' own -v or --verbose, or
// "" when there is none.
func commandName(args []string) string {
	for _, a := range args {
		if a != "-v" && a != "--verbose" && !strings.HasPrefix(a, "--verbose=") {
			return a
		}
	}
	return ""
}

// passThrough reports whether the arguments belong to claude agents: no
// arguments, a first argument of --, or a first argument that is not one of
// claudes' own. The -- form keeps working when claudes adds a command that
// claude agents also has.
func passThrough(root *cobra.Command, args []string) bool {
	if len(args) == 0 || args[0] == "--" {
		return true
	}
	first := commandName(args)
	switch first {
	case "", "-h", "--help", "help", "--version", "completion",
		cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return false
	}
	for _, c := range root.Commands() {
		if c.Name() == first || slices.Contains(c.Aliases, first) {
			return false
		}
	}
	return true
}

func (app *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "claudes",
		Short: "A wrapper around claude agents",
		Long: `claudes lists, pins, restarts, pauses, and resumes Claude Code sessions.

Run it with no command, or with arguments that are not a claudes command, and it
runs claude agents with those arguments. claudes -- ARGS always does, even when
ARGS starts with a claudes command.

Bulk commands leave pinned sessions alone unless you name them. Pins are the
same as Ctrl+T in agent view.`,
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
				if err := app.setLogLevel("debug"); err != nil {
					return err
				}
			}
			app.log().Debug("config", "path", app.ConfigPath)
			app.startReport(cmd)
			return app.lock(cmd)
		},
	}
	root.PersistentFlags().BoolP("verbose", "v", false, "log each claude call and step to stderr")
	root.AddCommand(
		app.lsCommand(),
		alone(reports(app.pinCommand(true))),
		alone(reports(app.pinCommand(false))),
		alone(reports(app.restartCommand())),
		alone(reports(app.pauseCommand())),
		alone(reports(app.resumeCommand())),
		alone(reports(app.pingCommand())),
		alone(reports(app.stopCommand())),
		app.logsCommand(),
		app.handOffCommand("attach", "Open a background session in this terminal", false),
		app.whereCommand(),
		alone(reports(app.gcCommand())),
		app.skillCommand(),
		app.configCommand(),
		app.doctorCommand(),
		app.sessionCommand(),
		app.teamCommand(),
		app.mcpCommand(),
		app.modCommand(),
		app.cdCommand(),
		app.initCommand(),
		app.upgradeCommand(),
	)
	// pwsh names PowerShell here, as in claudes init pwsh. The completion
	// scripts write to the output set when they are made, so set it first.
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	root.InitDefaultCompletionCmd()
	if ps, _, err := root.Find([]string{"completion", "powershell"}); err == nil && ps.Name() == "powershell" {
		ps.Aliases = append(ps.Aliases, "pwsh")
	}
	return root
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// say prints to stdout, unless the command prints a report there instead.
func (app *App) say(format string, args ...any) {
	if app.rep == nil {
		fmt.Fprintf(app.Out, format+"\n", args...)
	}
}

func (app *App) warn(format string, args ...any) {
	fmt.Fprintf(app.Err, format+"\n", args...)
	app.record(func(r *report) {
		if !r.printed {
			r.Warnings = append(r.Warnings, strings.TrimPrefix(fmt.Sprintf(format, args...), "claudes: "))
		}
	})
}

// errorLine prints a line to stderr, in red when stderr shows colors.
func (app *App) errorLine(format string, args ...any) {
	fmt.Fprintln(app.Err, app.errColors.Red(fmt.Sprintf(format, args...)))
}

func (app *App) confirmed(question string) bool {
	prompt := app.Out
	if app.rep != nil {
		prompt = app.Err
	}
	fmt.Fprintf(prompt, "%s [y/N] ", question)
	line, err := bufio.NewReader(app.In).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// listed lists the sessions a command will act on and notes when.
func (app *App) listed(ctx context.Context, all bool) ([]claude.Session, error) {
	before := app.Now()
	sessions, err := app.Agents.Sessions(ctx, all)
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

func (app *App) isHost(s claude.Session) bool {
	return app.isHostID(s.SessionID) || app.HostJob != "" && s.JobID == app.HostJob
}

// isHostID reports whether sessionID is the session running claudes.
func (app *App) isHostID(sessionID string) bool {
	return app.HostID != "" && sessionID == app.HostID ||
		app.HostJob != "" && claude.ShortID(sessionID) == app.HostJob
}

// hostLast moves the session running claudes to the end.
func (app *App) hostLast(sessions []claude.Session) []claude.Session {
	slices.SortStableFunc(sessions, func(a, b claude.Session) int {
		return trueFirst(!app.isHost(a), !app.isHost(b))
	})
	return sessions
}
