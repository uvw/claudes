package cli

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/uvw/claudes/internal/accounts"
	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/cswap"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/procs"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
	"github.com/uvw/claudes/internal/trash"
	"github.com/uvw/claudes/internal/upgrade"
)

const (
	defaultPing  = "Claude Code was restarted while you were working. Pick up where you left off."
	pollInterval = 15 * time.Second
)

// App is everything the commands touch, so tests can swap it.
type App struct {
	Agents claude.Agents
	// Found is the accounts claudes manages, and Accounts are the same with
	// the stores claudes reads in each, the one claudes runs as first.
	Found    accounts.Found
	Accounts []*account
	// only are the accounts --account names, or nil for all of them.
	only        []*account
	accountFlag string
	// CswapList runs `cswap list --json` with the cswap at binary, for
	// claudes doctor.
	CswapList func(ctx context.Context, binary string) ([]byte, error)
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
	// Exec hands the terminal to binary, run with config's CLAUDE_CONFIG_DIR.
	Exec func(config claude.ConfigDir, binary string, args ...string) error
	// Transcripts is ~/.claude/projects, and those of the other accounts, read
	// by claudes session.
	Transcripts transcripts.Store
	// Trash moves a file or directory to the system trash, and returns where
	// it went, or "" when the system does not say.
	Trash func(path string) (string, error)
	// Trashed records where each file Trash moved came from, for session
	// restore; one with no Path records nothing.
	Trashed trash.Manifest
	// trashedAt is when this run first moved something to the trash, which
	// groups what it moved in Trashed.
	trashedAt time.Time
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
	colors, errColors render.Palette
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
		CswapList:      cswap.List,
		Edit:           config.Edit,
		Interrupt:      signalContext,
		Which:          exec.LookPath,
		Chdir:          os.Chdir,
		HTTP:           &http.Client{Timeout: 5 * time.Minute},
		ReleasesURL:    upgrade.Releases,
		Executable:     os.Executable,
		HostID:         os.Getenv("CLAUDE_CODE_SESSION_ID"),
		HostJob:        jobs.IDFromDir(os.Getenv("CLAUDE_JOB_DIR")),
		Environ:        os.Environ,
	}
	app.Provenance = func(ctx context.Context, archive string, fromGitHub bool) (string, error) {
		return upgrade.Provenance(ctx, app.Which, archive, fromGitHub)
	}
	app.Width = func() int { return render.TerminalWidth(app.Out) }
	client := claude.NewClient()
	client.Log = app.log()
	app.Agents = client
	var parkErr error
	_, homeErr := paths.ClaudeHome()
	app.Park, parkErr = park.DefaultFile()
	app.setupErr = errors.Join(homeErr, parkErr)
	if parkErr == nil {
		app.LockPath = filepath.Join(filepath.Dir(app.Park.Path), "run.lock")
	}
	// A missing Claude Code directory is setupErr's to report.
	app.Transcripts, _ = transcripts.DefaultStore()
	// Without a state directory, the park file's error stops every command.
	app.Trashed, _ = trash.DefaultManifest()
	// Without a cache directory, claudes asks claude for its version each time.
	app.CacheDir, _ = paths.Cache()
	if app.ConfigPath, app.configErr = config.Path(); app.configErr == nil {
		app.Config, app.configErr = config.Load(app.ConfigPath)
	}
	app.findAccounts()
	return app
}

// remover is how a command removes a file or directory: it moves it to the
// system trash and adds where it went to rec, or with --purge deletes it for
// good.
func (app *App) remover(purge bool, rec *trash.Record) func(string) error {
	if purge {
		return os.RemoveAll
	}
	return func(path string) error {
		to, err := app.Trash(path)
		if err == nil {
			rec.Files = append(rec.Files, trash.File{From: path, To: to})
		}
		return err
	}
}

// keepTrashed records what command moved to the trash for one session, so
// session restore can put it back. It records even after a removal failed
// partway, since what moved before the failure is in the trash.
func (app *App) keepTrashed(command string, rec trash.Record) {
	if len(rec.Files) == 0 {
		return
	}
	if app.trashedAt.IsZero() {
		app.trashedAt = app.Now()
	}
	if err := app.Trashed.Add(app.trashedAt, command, rec); err != nil {
		app.warn("claudes: moved %s to the trash, but cannot record where, so session restore cannot put it back: %v", rec.Name, err)
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
