// Package e2e runs the compiled claudes binary against fakeclaude, a stand-in
// for the claude command line, with every directory claudes touches moved into a
// temporary directory.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/uvw/claudes/test/e2e/internal/fake"
)

// binEnv names a directory that already holds claudes and claude, built for this
// platform, so the tests can run where there is no Go toolchain, such as a
// cross-compiled test binary in WSL.
const binEnv = "CLAUDES_E2E_BIN"

var binDir string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	binDir = os.Getenv(binEnv)
	if binDir == "" {
		tmp, err := os.MkdirTemp("", "claudes-e2e-")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer os.RemoveAll(tmp)
		binDir = tmp
		for pkg, name := range map[string]string{
			"github.com/uvw/claudes/cmd/claudes":         "claudes",
			"github.com/uvw/claudes/test/e2e/fakeclaude": "claude",
		} {
			out, err := exec.Command("go", "build", "-o", filepath.Join(binDir, name+exe()), pkg).CombinedOutput()
			if err != nil {
				fmt.Fprintf(os.Stderr, "go build %s: %v\n%s", pkg, err, out)
				return 1
			}
		}
	}
	return m.Run()
}

func exe() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// world is one test's machine: a home directory, a Claude Code directory, a state
// directory for the park file, and the fake's sessions.
type world struct {
	t *testing.T
	// claudeDir is CLAUDE_CONFIG_DIR and stateHome is XDG_STATE_HOME.
	root, home, claudeDir, stateHome string
	statePath, logPath               string
	env                              []string
	installed                        string
	failures                         map[string]string
	agentsExit                       int
	sessions                         []fake.Session
	started                          bool
	oneHourAgo                       int64
}

func newWorld(t *testing.T) *world {
	t.Helper()
	root := t.TempDir()
	w := &world{
		t:          t,
		root:       root,
		home:       filepath.Join(root, "home"),
		claudeDir:  filepath.Join(root, "home", "claude-config"),
		stateHome:  filepath.Join(root, "home", "state"),
		statePath:  filepath.Join(root, "fake", "state.json"),
		logPath:    filepath.Join(root, "fake", "calls.jsonl"),
		installed:  "2.1.325",
		oneHourAgo: time.Now().Add(-time.Hour).UnixMilli(),
	}
	for _, dir := range []string{w.home, w.jobsDir(), w.stateHome, filepath.Dir(w.statePath)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func jobID(name string) string { return fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(name))) }

// sessionID differs from the job id, as it does in real use, so passing one
// where the other belongs fails.
func sessionID(name string) string {
	return fmt.Sprintf("%08x-1111-2222-3333-444455556666", crc32.ChecksumIEEE([]byte("session/"+name)))
}

// directory makes a session directory. Its name has a space and a quote, so a resume
// in it must survive quoting on every platform.
func (w *world) folder(name string) string {
	dir := filepath.Join(w.root, "work dir", "it's "+name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.t.Fatal(err)
	}
	return dir
}

// bg adds a running background session. PIDs are above any real pid_max.
func (w *world) bg(name, status string) fake.Session {
	s := fake.Session{
		ID: jobID(name), SessionID: sessionID(name), Name: name, Cwd: w.folder(name),
		Kind: "background", State: stateFor(status), Status: status, PID: 8_000_000 + len(w.sessions),
		StartedAt: w.oneHourAgo - int64(len(w.sessions))*1000,
	}
	w.sessions = append(w.sessions, s)
	return s
}

// tty adds a running interactive session, which has no job id.
//
//nolint:unparam // Named at the call, as bg is, so the test shows the name.
func (w *world) tty(name string) fake.Session {
	s := fake.Session{
		SessionID: sessionID(name), Name: name, Cwd: w.folder(name), Kind: "interactive",
		Status: "idle", PID: 8_000_000 + len(w.sessions), StartedAt: w.oneHourAgo,
	}
	w.sessions = append(w.sessions, s)
	return s
}

// finished adds a stopped background session that started ago.
func (w *world) finished(name, state string, ago time.Duration) fake.Session {
	s := fake.Session{
		ID: jobID(name), SessionID: sessionID(name), Name: name, Cwd: w.folder(name),
		Kind: "background", State: state, StartedAt: time.Now().Add(-ago).UnixMilli(),
	}
	w.sessions = append(w.sessions, s)
	return s
}

func (w *world) jobsDir() string { return filepath.Join(w.claudeDir, "jobs") }

func (w *world) parkPath() string { return filepath.Join(w.stateHome, "claudes", "parked.json") }

// configHome is XDG_CONFIG_HOME, so config.yaml is configHome/claudes.
func (w *world) configHome() string { return filepath.Join(w.home, "config") }

func (w *world) writeFile(path, data string) {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		w.t.Fatal(err)
	}
}

// pin writes pins.json the way agent view does.
func (w *world) pin(sessions ...fake.Session) {
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	data, _ := json.MarshalIndent(ids, "", "  ")
	w.writeFile(filepath.Join(w.jobsDir(), "pins.json"), string(data))
}

// jobVersion records the Claude Code version a background session last ran.
func (w *world) jobVersion(s fake.Session, version string) {
	w.writeFile(filepath.Join(w.jobsDir(), s.ID, "state.json"), `{"cliVersion":"`+version+`"}`)
}

// transcript writes session id's transcript, one JSON record per line, where
// Claude Code keeps it for a session that ran in dir.
func (w *world) transcript(dir, id string, records ...map[string]string) {
	w.t.Helper()
	var b strings.Builder
	for _, r := range records {
		line, err := json.Marshal(r)
		if err != nil {
			w.t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	w.writeFile(filepath.Join(w.claudeDir, "projects", folderName(dir), id+".jsonl"), b.String())
}

func (w *world) start() {
	w.t.Helper()
	if w.started {
		return
	}
	w.started = true
	data, err := json.MarshalIndent(fake.State{
		Version: w.installed, AgentsExit: w.agentsExit, Fail: w.failures, Sessions: w.sessions,
	}, "", "  ")
	if err != nil {
		w.t.Fatal(err)
	}
	w.writeFile(w.statePath, string(data))
}

type result struct {
	stdout, stderr string
	code           int
}

// environ is the test process's environment with every Claude Code variable
// and every directory claudes reads replaced, and the color variables dropped.
// PATH holds only the fake, so the real claude can never be found.
func (w *world) environ() []string {
	drop := []string{"PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "PWD", "NO_COLOR", "CLICOLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "TTY_FORCE"}
	var env []string
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		upper := strings.ToUpper(name)
		if slices.Contains(drop, upper) || strings.HasPrefix(upper, "XDG_") || strings.HasPrefix(upper, "CLAUDE") || strings.HasPrefix(upper, "FAKE_CLAUDE") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env,
		"PATH="+binDir,
		"HOME="+w.home,
		"USERPROFILE="+w.home,
		"CLAUDE_CONFIG_DIR="+w.claudeDir,
		"XDG_STATE_HOME="+w.stateHome,
		"XDG_CONFIG_HOME="+w.configHome(),
		"XDG_CACHE_HOME="+filepath.Join(w.home, "cache"),
		"APPDATA="+filepath.Join(w.home, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(w.home, "AppData", "Local"),
		"FAKE_CLAUDE_STATE="+w.statePath,
		"FAKE_CLAUDE_LOG="+w.logPath,
	)
	return append(env, w.env...)
}

// run runs the compiled claudes and returns what it printed and its exit code.
func (w *world) run(args ...string) result {
	w.t.Helper()
	w.start()
	ctx, cancel := context.WithTimeout(w.t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(binDir, "claudes"+exe()), args...)
	cmd.Dir = w.root
	cmd.Env = w.environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	r := result{stdout: stdout.String(), stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		r.code = exit.ExitCode()
	case err != nil:
		w.t.Fatalf("claudes %q: %v", args, err)
	}
	return r
}

func (w *world) ok(args ...string) result {
	w.t.Helper()
	r := w.run(args...)
	if r.code != 0 {
		w.t.Fatalf("claudes %q exited %d\nstdout:\n%s\nstderr:\n%s", args, r.code, r.stdout, r.stderr)
	}
	return r
}

// calls reads every call claudes made to the fake.
func (w *world) calls() []fake.Call {
	w.t.Helper()
	data, err := os.ReadFile(w.logPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		w.t.Fatal(err)
	}
	var out []fake.Call
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var c fake.Call
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			w.t.Fatalf("calls log line %q: %v", line, err)
		}
		out = append(out, c)
	}
	return out
}

// actions are the calls that change something: everything but listing and
// the version check.
func (w *world) actions() []fake.Call {
	return slices.DeleteFunc(w.calls(), func(c fake.Call) bool {
		return len(c.Args) >= 2 && c.Args[0] == "agents" && c.Args[1] == "--json" ||
			len(c.Args) == 1 && c.Args[0] == "--version"
	})
}

func (w *world) resetCalls() {
	w.t.Helper()
	if err := os.Remove(w.logPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		w.t.Fatal(err)
	}
}

// state reads the fake's sessions back.
func (w *world) state() map[string]fake.Session {
	w.t.Helper()
	data, err := os.ReadFile(w.statePath)
	if err != nil {
		w.t.Fatal(err)
	}
	var st fake.State
	if err := json.Unmarshal(data, &st); err != nil {
		w.t.Fatal(err)
	}
	out := map[string]fake.Session{}
	for _, s := range st.Sessions {
		out[s.Name] = s
	}
	return out
}

func argsOf(calls []fake.Call) [][]string {
	out := make([][]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.Args)
	}
	return out
}

func assertArgs(t *testing.T, calls []fake.Call, want ...[]string) {
	t.Helper()
	if diff := cmp.Diff(want, argsOf(calls)); diff != "" {
		t.Fatalf("calls (-want +got):\n%s", diff)
	}
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("output does not contain %q:\n%s", want, text)
	}
}

// assertIn checks that a call ran in dir. Paths are compared as files, since
// a temporary directory can be reached through a symlink, as on macOS.
func assertIn(t *testing.T, c fake.Call, dir string) {
	t.Helper()
	a, errA := os.Stat(c.Cwd)
	b, errB := os.Stat(dir)
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Fatalf("%q ran in %q, want %q", c.Args, c.Cwd, dir)
	}
}

// stateFor is the state claude agents shows with a status: a session in a
// turn is working, and one between turns is done.
func stateFor(status string) string {
	if status == "busy" || status == "waiting" || status == "shell" {
		return "working"
	}
	return "done"
}
