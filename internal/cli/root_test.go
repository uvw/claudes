package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
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
	"claudes unpin": "pinCommand builds pin and unpin, which differ only in what they write",
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

// appFile is the name of a file of plumbing the commands share, which may end
// in _unix or _windows.
var appFile = regexp.MustCompile(`^app_[a-z]+(_unix|_windows)?$`)

// commandFiles are the names, without .go, that a command's file takes: its
// path below claudes, joined by underscores, such as session_mv.
func commandFiles(t *testing.T) map[string]bool {
	t.Helper()
	files := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			// cobra's own help and completion commands.
			if c.Name() != "help" && c.Name() != "completion" {
				walk(c)
			}
		}
		if cmd.HasParent() {
			files[strings.ReplaceAll(strings.TrimPrefix(cmd.CommandPath(), "claudes "), " ", "_")] = true
		}
	}
	walk(newHarness(t).app.rootCommand())
	return files
}

func TestEveryFileIsACommandAnAppFileOrRoot(t *testing.T) {
	commands := commandFiles(t)
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		base, test := strings.CutSuffix(strings.TrimSuffix(name, ".go"), "_test")
		fits := base == "root" || base == "app" || appFile.MatchString(base) || commands[base] ||
			test && base == "harness"
		if !fits {
			t.Errorf("%s is not root, app, app_<concern>, or a command's file: "+
				"move its code to one of those, or to a package of its own concern", name)
		}
	}
}

func TestPassThrough(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"claude", "agents"}},
		{[]string{"--cwd", ".", "--model", "opus"}, []string{"claude", "agents", "--cwd", ".", "--model", "opus"}},
		{[]string{"--"}, []string{"claude", "agents"}},
		{[]string{"--", "ls", "--json"}, []string{"claude", "agents", "ls", "--json"}},
		{[]string{"--", "--", "x"}, []string{"claude", "agents", "--", "x"}},
	} {
		h := newHarness(t)
		h.ok(t, tc.args...)
		if len(h.execs) != 1 || !slices.Equal(h.execs[0], tc.want) {
			t.Fatalf("%v: execs %q", tc.args, h.execs)
		}
	}
}

func TestOwnCommandsAndHelpDoNotPassThrough(t *testing.T) {
	for _, args := range [][]string{{"ls"}, {"list"}, {"--help"}, {"help"}, {"--version"}} {
		h := newHarness(t, nil)
		h.ok(t, args...)
		if len(h.execs) != 0 {
			t.Fatalf("%v passed through", args)
		}
	}
}

func TestAMissingHomeFolderStopsEveryCommand(t *testing.T) {
	for _, name := range []string{"HOME", "USERPROFILE", "CLAUDE_CONFIG_DIR", "XDG_STATE_HOME"} {
		t.Setenv(name, "")
	}
	app := newApp()
	var errOut bytes.Buffer
	app.Err = &errOut
	app.Agents = &fakeAgents{}
	if code := app.Run([]string{"restart"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	assertContains(t, errOut.String(), "cannot find")
}

func TestExitCodesSayWhatHappened(t *testing.T) {
	h := newHarness(t)
	one, two := h.bg("review-one", "idle"), h.bg("review-two", "idle")
	bad := h.bg("bad", "idle")
	h.agents.snapshots = [][]claude.Session{{one, two, bad}}

	h.exits(t, exitNoMatch, "stop", "nothing-like-it")
	h.exits(t, exitNoMatch, "stop", "review")

	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}
	h.exits(t, exitSomeFailed, "stop", "bad")

	odd := h.bg("odd", "idle")
	odd.Kind = "daemon"
	h.agents.snapshots = [][]claude.Session{{odd}}
	h.exits(t, exitRefused, "stop", "odd")
}

func TestVerboseFirstStillRunsClaudesCommands(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	for _, flag := range []string{"-v", "--verbose", "--verbose=true"} {
		h.out.Reset()
		h.ok(t, flag, "ls")
		assertContains(t, h.out.String(), "api")
	}
	if len(h.execs) != 0 {
		t.Fatalf("passed through to claude agents: %q", h.execs)
	}
}

func TestPassThroughThatCannotRunClaude(t *testing.T) {
	h := newHarness(t)
	h.app.Exec = func(string, ...string) error { return errors.New("executable file not found") }

	h.fails(t, "--some-agents-flag")
	assertContains(t, h.err.String(), "claudes: cannot run claude agents: executable file not found")
}
