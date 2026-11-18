package cli

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

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
