package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
)

// inParentsFile are the commands whose code lives in another command's file,
// each with why.
var inParentsFile = map[string]string{
	"claudes unpin":        "pinCommand builds pin and unpin, which differ only in what they write",
	"claudes config path":  "a few lines, so it stays in config.go",
	"claudes config get":   "a few lines, so it stays in config.go",
	"claudes config set":   "a few lines, so it stays in config.go",
	"claudes mcp profiles": "a few lines, so it stays in mcp.go",
}

// TestEachCommandLivesInItsFile walks the command tree and finds the file of
// each command's RunE, so a command's code is where its name says:
// session_mv.go for claudes session mv.
func TestEachCommandLivesInItsFile(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			walk(c)
		}
		if cmd.RunE == nil || !cmd.HasParent() {
			return
		}
		file, _ := runtime.FuncForPC(reflect.ValueOf(cmd.RunE).Pointer()).FileLine(reflect.ValueOf(cmd.RunE).Pointer())
		if dir, _ := filepath.Abs("."); filepath.Dir(file) != dir {
			return // cobra's own, such as help and completion
		}
		path := cmd.CommandPath()
		want := strings.ReplaceAll(strings.TrimPrefix(path, "claudes "), " ", "_") + ".go"
		if filepath.Base(file) == want {
			if _, ok := inParentsFile[path]; ok {
				t.Errorf("%s is in %s now: take it off inParentsFile", path, want)
			}
			return
		}
		if _, ok := inParentsFile[path]; !ok {
			t.Errorf("%s runs from %s, not %s: move it, or add it to inParentsFile with why", path, filepath.Base(file), want)
		}
	}
	walk(newHarness(t).app.rootCommand())
}

type panicky struct{ *fakeAgents }

func (p panicky) Respawn(context.Context, string) error { panic("boom") }

func TestAPanicFreesTheRunLock(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	h.app.Agents = panicky{h.agents}
	func() {
		defer func() { _ = recover() }()
		h.run("restart", "api")
	}()
	if _, err := os.Stat(h.app.LockPath); err == nil {
		t.Fatal("the run lock outlived a panic")
	}
}

// docs/reference.md: exit 4 is "Refused before any change: the wrong kind of
// session, or a failed safety check".
func TestSafetyRefusalsExitFour(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "busy")
	live := h.bg("live", "busy")
	h.app.HostID = host.SessionID
	h.agents.snapshots = [][]claude.Session{{host, live}}
	h.transcriptAs(t, live.SessionID, "live", live.Cwd, time.Minute)
	for _, args := range [][]string{
		{"ping", "host", "hi"},
		{"session", "resume", "live"},
		{"session", "mv", "live", h.folder("other")},
		{"gc", "live"},
	} {
		os.MkdirAll(h.folder("other"), 0o755)
		h.err.Reset()
		if code := h.run(args...); code != exitRefused {
			t.Errorf("%v exits %d: %s", args, code, strings.TrimSpace(h.err.String()))
		}
	}
}

func TestBuildVersionPrefersTheReleaseVersion(t *testing.T) {
	defer func(v, c string) { Version, Commit = v, c }(Version, Commit)
	Version, Commit = "0.3.0", "abc1234"
	if got := buildVersion(); got != "0.3.0 (abc1234)" {
		t.Fatalf("got %q", got)
	}
	Commit = ""
	if got := buildVersion(); got != "0.3.0" {
		t.Fatalf("got %q", got)
	}
}

// A claudes suspended mid-run, such as by a closed laptop lid, stops
// refreshing the run lock. The lock names its process, so it stays while that
// process runs, and goes once it has ended.
func TestTheRunLockOutlivesASuspendButNotItsOwner(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	if err := os.MkdirAll(h.app.LockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.app.LockPath, "owner"), fmt.Sprintf("4242 %s\n", start.Format(time.RFC3339Nano)))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(h.app.LockPath, old, old); err != nil {
		t.Fatal(err)
	}
	h.app.Alive = func(pid int, started time.Time) bool { return pid == 4242 && started.Equal(start) }

	h.exits(t, exitBusy, "stop", "api")
	assertCalls(t, h)

	h.app.Alive = func(int, time.Time) bool { return false }
	h.ok(t, "stop", "api")
	assertCalls(t, h, "stop "+s.JobID)
}

func TestTheRunLockNamesItsProcess(t *testing.T) {
	h := newHarness(t)
	owner := h.app.lockOwner()
	h.app.Alive = func(pid int, started time.Time) bool { return pid == os.Getpid() && started.Equal(start) }
	if h.app.lockOwnerGone(owner) {
		t.Fatalf("owner %q of this process reads as gone", owner)
	}
	for _, bad := range []string{"", "x", "0 " + start.Format(time.RFC3339Nano), "42 yesterday"} {
		if !h.app.lockOwnerGone([]byte(bad)) {
			t.Errorf("owner %q reads as running", bad)
		}
	}
}

// maxHelpWidth is the widest line help may print, so it reads in an
// 80-column terminal without wrapping.
const maxHelpWidth = 80

func TestHelpFitsEightyColumns(t *testing.T) {
	h := newHarness(t)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			// cobra's own help and completion commands.
			if c.Name() != "help" && c.Name() != "completion" {
				walk(c)
			}
		}
		var b strings.Builder
		cmd.SetOut(&b)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(b.String()) {
			if line = strings.TrimRight(line, "\n"); utf8.RuneCountInString(line) > maxHelpWidth {
				t.Errorf("%s --help has a line of %d columns:\n%s", cmd.CommandPath(), utf8.RuneCountInString(line), line)
			}
		}
	}
	walk(h.app.rootCommand())
}
