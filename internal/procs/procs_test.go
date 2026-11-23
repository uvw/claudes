package procs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	ossignal "os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// helperEnv makes the test binary stand in for a session's processes:
// "sleep" sleeps, and "tree" starts a sleeping child, prints its PID, and
// sleeps.
const helperEnv = "CLAUDES_PROCS_HELPER"

func TestMain(m *testing.M) {
	RunHelper()
	switch os.Getenv(helperEnv) {
	case "breakable":
		// A session that saves its state on Ctrl-Break, which Go reports as
		// os.Interrupt, then exits.
		stop := make(chan os.Signal, 1)
		ossignal.Notify(stop, os.Interrupt)
		fmt.Println(os.Getpid())
		select {
		case <-stop:
			os.WriteFile(os.Getenv("CLAUDES_PROCS_MARKER"), []byte("stopped"), 0o644)
		case <-time.After(30 * time.Second):
		}
		os.Exit(0)
	case "launch":
		// A program that is not a shell, running a session on its console.
		child := exec.Command(os.Getenv("CLAUDES_PROCS_CHILD"))
		child.Env = append(os.Environ(), helperEnv+"=breakable")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "sleep":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "tree":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperEnv+"=sleep")
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Println(child.Process.Pid)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "host":
		// A claude whose tree holds the process that stops it, as when
		// claudes stops the session it runs in.
		tree := exec.Command(os.Args[0])
		tree.Env = append(os.Environ(), helperEnv+"=tree")
		pipe, err := tree.StdoutPipe()
		if err != nil || tree.Start() != nil {
			os.Exit(2)
		}
		var grand int
		fmt.Fscan(pipe, &grand)
		fmt.Println(grand)
		stopper := exec.Command(os.Args[0])
		stopper.Env = append(os.Environ(), helperEnv+"=stopper")
		stopper.Run()
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "stopper":
		Terminate(os.Getppid(), time.Time{}, time.Now())
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestIsClaudeExecutable(t *testing.T) {
	for file, want := range map[string]bool{
		"claude":                                         true,
		"/Users/me/.local/bin/claude":                    true,
		`C:\Users\me\.local\bin\claude.exe`:              true,
		`C:\Users\me\.local\bin\CLAUDE.EXE`:              true,
		"/Users/me/.local/share/claude/versions/2.1.325": true,
		"claudes":                                false,
		`C:\Users\me\.local\bin\claude-swap.exe`: false,
		`C:\Users\me\AppData\Local\AnthropicClaude\app-1.0\claude.exe`: false,
		"/Applications/Claude.app/Contents/MacOS/Claude":               false,
		"vim": false,
	} {
		if got := isClaudeExecutable(file); got != want {
			t.Errorf("%s: %v", file, got)
		}
	}
}

func TestIsClaudeCommand(t *testing.T) {
	for command, want := range map[string]bool{
		"claude --resume abc":         true,
		"/Users/me/.local/bin/claude": true,

		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js":                                                      true,
		`"C:\Program Files\nodejs\node.exe" C:\Users\me\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\cli.js`: true,
		"vim /Users/me/.claude/settings.json":                                                                              false,
		"sh -c sleep 60; echo ~/.claude/settings":                                                                          false,
		"claudes restart": false,
	} {
		if got := isClaudeCommand(command); got != want {
			t.Errorf("%q: %v", command, got)
		}
	}
}

func TestCheckStartRefusesAProcessStartedAfterTheList(t *testing.T) {
	listed := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	if err := checkStart(1, listed.Add(-time.Hour), time.Time{}, listed); err != nil {
		t.Fatalf("earlier process refused: %v", err)
	}
	if err := checkStart(1, listed.Add(time.Second), time.Time{}, listed); err == nil {
		t.Fatal("later process accepted")
	}
	if err := checkStart(1, listed.Add(time.Second), time.Time{}, time.Time{}); err != nil {
		t.Fatalf("unknown list time refused: %v", err)
	}
}

func TestCheckStartRefusesAProcessStartedAfterTheSession(t *testing.T) {
	session := time.Date(2026, 11, 17, 17, 14, 36, 104e6, time.Local)
	listed := session.Add(time.Hour)
	for _, tc := range []struct {
		name    string
		started time.Time
		ok      bool
	}{
		// Measured: an interactive session's process starts about 2 s before
		// its startedAt, and ps rounds down to the second.
		{"an interactive session's own process", session.Add(-2100 * time.Millisecond).Truncate(time.Second), true},
		{"a spare that waited hours for its job", session.Add(-8 * time.Hour), true},
		{"within the slack", session.Add(time.Second), true},
		{"a PID reused after the session started", session.Add(10 * time.Minute), false},
	} {
		err := checkStart(1, tc.started, session, listed)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err %v", tc.name, err)
		}
	}
}

type helper struct {
	pid    int
	out    io.Reader
	kill   func()
	exited <-chan struct{}
}

// start runs a copy of the test binary under name in mode, with args. It reaps
// the process as soon as it exits, and kills it when the test ends.
func start(t *testing.T, name, mode string, args ...string) helper {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	kill := func() {
		cmd.Process.Kill()
		<-exited
	}
	t.Cleanup(kill)
	// Start returns once exec is past the point of no return, which on Linux
	// is before the new command line is readable.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, ok := Started(cmd.Process.Pid); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s never showed up as running", name)
		}
	}
	return helper{cmd.Process.Pid, out, kill, exited}
}

// npmPackage is Claude Code's entry point in an npm install. It uses the
// platform's separators, so Windows checks a command line with backslashes.
var npmPackage = filepath.Join("node_modules", "@anthropic-ai", "claude-code", "cli.js")

func TestTerminateRefusesAProcessThatIsNotClaude(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"not-claude", nil},
		{"node", []string{filepath.Join("node_modules", "vite", "bin", "vite.js")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := start(t, tc.name, "sleep", tc.args...)
			if err := Terminate(h.pid, time.Time{}, time.Now()); err == nil || !strings.Contains(err.Error(), "not a claude process") {
				t.Fatalf("err %v", err)
			}
			if !Alive(h.pid) {
				t.Fatal("the process was killed")
			}
		})
	}
}

func TestTerminateRefusesAReusedPID(t *testing.T) {
	h := start(t, "claude", "sleep")
	if err := Terminate(h.pid, time.Time{}, time.Now().Add(-time.Hour)); err == nil || !strings.Contains(err.Error(), "started after") {
		t.Fatalf("err %v, want a process that started after the list refused", err)
	}
	// The session started an hour ago, and the list is now: the PID was
	// reused before claudes listed the sessions.
	if err := Terminate(h.pid, time.Now().Add(-time.Hour), time.Now()); err == nil || !strings.Contains(err.Error(), "after the session did") {
		t.Fatalf("err %v, want a process that started after the session refused", err)
	}
	if !Alive(h.pid) {
		t.Fatal("the process was killed")
	}
}

func TestTerminateStopsClaudeAndItsChildren(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"claude", []string{"--resume", "abc"}},
		{"node", []string{npmPackage, "--resume", "abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := start(t, tc.name, "tree", tc.args...)
			var childPID int
			if _, err := fmt.Fscan(h.out, &childPID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if p, err := os.FindProcess(childPID); err == nil {
					p.Kill()
				}
			})
			if err := Terminate(h.pid, time.Time{}, time.Now()); err != nil {
				t.Fatal(err)
			}
			select {
			case <-h.exited:
			case <-time.After(5 * time.Second):
				t.Fatalf("%s is still running", tc.name)
			}
			deadline := time.Now().Add(5 * time.Second)
			for Alive(childPID) && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			if Alive(childPID) {
				t.Fatal("the child is still running")
			}
		})
	}
}

func TestTerminateOfAProcessThatIsGoneIsNotAnError(t *testing.T) {
	h := start(t, "claude", "sleep")
	h.kill()
	if err := Terminate(h.pid, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestIsClaudeCommandWithASpaceInThePath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Jane Doe", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(dir, "claude")
	if err := os.WriteFile(claude, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if !isClaudeCommand(claude + " --resume abc") {
		t.Error("claude under a directory with a space was refused")
	}
	if isClaudeCommand(filepath.Join(dir, "vim") + " " + claude) {
		t.Error("an editor opening a file named claude was taken for claude")
	}
	if isClaudeCommand("vim /no such/claude") {
		t.Error("a missing path ending in claude was accepted")
	}
}

func TestTerminateFromInsideTheTreeStopsEveryOtherDescendant(t *testing.T) {
	h := start(t, "claude", "host")
	var grand int
	if _, err := fmt.Fscan(h.out, &grand); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(grand); err == nil {
			p.Kill()
		}
	})
	select {
	case <-h.exited:
	case <-time.After(15 * time.Second):
		t.Fatal("claude is still running")
	}
	deadline := time.Now().Add(5 * time.Second)
	for Alive(grand) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if Alive(grand) {
		t.Fatalf("process %d, below claude, outlived the stop", grand)
	}
}

func TestRunningNoticesAReusedPID(t *testing.T) {
	h := start(t, "claude", "sleep")
	started, ok := Started(h.pid)
	if !ok || !Running(h.pid, started) || !Running(h.pid, time.Time{}) {
		t.Fatalf("started %v, ok %v", started, ok)
	}
	if Running(h.pid, started.Add(-time.Hour)) {
		t.Fatal("a process with another start time counted as the same one")
	}
	h.kill()
	if Running(h.pid, started) {
		t.Fatal("a process that exited still counts")
	}
}

func TestStartedIsWhenTheProcessStarted(t *testing.T) {
	before := time.Now()
	h := start(t, "claude", "sleep")
	after := time.Now()
	started, ok := Started(h.pid)
	if !ok {
		t.Fatal("the process is not running")
	}
	// ps gives whole seconds, so a start can read up to a second early.
	earliest, latest := before.Add(-time.Second), after.Add(listSlack)
	if started.Before(earliest) || started.After(latest) {
		t.Fatalf("started %v, want between %v and %v", started, earliest, latest)
	}
}

func TestVersionOf(t *testing.T) {
	for path, want := range map[string]string{
		"/Users/me/.local/share/claude/versions/2.1.326":       "2.1.326",
		"/home/me/.local/share/claude/versions/2.1.326":        "2.1.326",
		`C:\Users\me\.local\share\claude\versions\2.1.326.exe`: "2.1.326",
		"/opt/homebrew/bin/node":                               "",
		"/usr/local/bin/claude":                                "",
		"/Users/me/.local/share/claude/versions/2.1.326/extra": "",
		"/Users/me/notclaude/versions/2.1.326":                 "",
	} {
		if got := VersionOf(path); got != want {
			t.Errorf("VersionOf(%q) = %q, want %q", path, got, want)
		}
	}
}
