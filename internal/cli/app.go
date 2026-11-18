package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/procs"
	"github.com/uvw/claudes/internal/selfupdate"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/teams"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
)

const (
	defaultPing  = "Claude Code was restarted while you were working. Pick up where you left off."
	pollInterval = 15 * time.Second
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
	// MCP is where Claude Code keeps MCP servers, which claudes mcp reads, and
	// the one reader of ~/.claude.json, whose feature flags doctor reports.
	MCP mcp.Store
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
	// Interrupt returns a context that the first Ctrl-C, or on Unix SIGTERM or
	// SIGHUP, cancels, and the function that restores the default, so a second
	// one ends claudes.
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
		Interrupt:      interruptContext,
		Which:          exec.LookPath,
		Chdir:          os.Chdir,
		HTTP:           &http.Client{Timeout: 5 * time.Minute},
		ReleasesURL:    selfupdate.Releases,
		Executable:     os.Executable,
		HostID:         os.Getenv("CLAUDE_CODE_SESSION_ID"),
		HostJob:        hostJob(os.Getenv("CLAUDE_JOB_DIR")),
		Environ:        os.Environ,
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
	app.MCP, _ = mcp.DefaultStore()
	// Without a cache directory, claudes asks claude for its version each time.
	app.CacheDir, _ = paths.Cache()
	if app.ConfigPath, app.configErr = config.Path(); app.configErr == nil {
		app.Config, app.configErr = config.Load(app.ConfigPath)
	}
	return app
}

// hostJob is the job id a CLAUDE_JOB_DIR names, such as ff78d735 for
// ~/.claude/jobs/ff78d735, or "" when it names none.
func hostJob(dir string) string {
	if id := filepath.Base(filepath.Clean(dir)); dir != "" && jobs.ValidID(id) {
		return id
	}
	return ""
}

// interruptContext is Interrupt: ctx, canceled by the first of stopSignals.
func interruptContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, stopSignals...)
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

// configPathErr is the error when claudes cannot even tell where config.yaml
// is, such as a relative $CLAUDES_CONFIG.
func (app *App) configPathErr() error {
	if app.ConfigPath == "" {
		return app.configErr
	}
	return nil
}
