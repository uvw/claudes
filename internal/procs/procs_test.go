package procs

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
	switch os.Getenv(helperEnv) {
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
		Terminate(os.Getppid(), time.Now())
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

		"node /usr/lib/node_modules/@anthropic-ai/claude-code/cli.js": true,
		"vim /Users/me/.claude/settings.json":                         false,
		"sh -c sleep 60; echo ~/.claude/settings":                     false,
		"claudes restart": false,
	} {
		if got := isClaudeCommand(command); got != want {
			t.Errorf("%q: %v", command, got)
		}
	}
}

func TestCheckStartRefusesAProcessStartedAfterTheList(t *testing.T) {
	listed := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	if err := checkStart(1, listed.Add(-time.Hour), listed); err != nil {
		t.Fatalf("earlier process refused: %v", err)
	}
	if err := checkStart(1, listed.Add(time.Second), listed); err == nil {
		t.Fatal("later process accepted")
	}
	if err := checkStart(1, listed.Add(time.Second), time.Time{}); err != nil {
		t.Fatalf("unknown list time refused: %v", err)
	}
}

type helper struct {
	pid    int
	out    io.Reader
	kill   func()
	exited <-chan struct{}
}

// start runs a copy of the test binary under name in mode. It reaps the
// process as soon as it exits, and kills it when the test ends.
func start(t *testing.T, name, mode string) helper {
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
	cmd := exec.Command(bin)
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
	return helper{cmd.Process.Pid, out, kill, exited}
}

func TestTerminateRefusesAProcessThatIsNotClaude(t *testing.T) {
	h := start(t, "not-claude", "sleep")
	if err := Terminate(h.pid, time.Now()); err == nil || !strings.Contains(err.Error(), "not a claude process") {
		t.Fatalf("err %v", err)
	}
	if !Alive(h.pid) {
		t.Fatal("the process was killed")
	}
}

func TestTerminateRefusesAReusedPID(t *testing.T) {
	h := start(t, "claude", "sleep")
	if err := Terminate(h.pid, time.Now().Add(-time.Hour)); err == nil || !strings.Contains(err.Error(), "started after") {
		t.Fatalf("err %v, want a process that started after the list refused", err)
	}
	if !Alive(h.pid) {
		t.Fatal("the process was killed")
	}
}

func TestTerminateStopsClaudeAndItsChildren(t *testing.T) {
	h := start(t, "claude", "tree")
	var childPID int
	if _, err := fmt.Fscan(h.out, &childPID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p, err := os.FindProcess(childPID); err == nil {
			p.Kill()
		}
	})
	if err := Terminate(h.pid, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("claude is still running")
	}
	deadline := time.Now().Add(5 * time.Second)
	for Alive(childPID) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if Alive(childPID) {
		t.Fatal("the child is still running")
	}
}

func TestTerminateOfAProcessThatIsGoneIsNotAnError(t *testing.T) {
	h := start(t, "claude", "sleep")
	h.kill()
	if err := Terminate(h.pid, time.Now()); err != nil {
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
		t.Error("claude under a folder with a space was refused")
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
