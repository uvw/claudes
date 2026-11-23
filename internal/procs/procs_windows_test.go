package procs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// copyAs is the test binary under another name in a temporary directory.
func copyAs(t *testing.T, name string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), name+".exe")
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// onOwnConsole starts bin in mode on a hidden console of its own, so
// Ctrl-Break sent to it never reaches the test, and returns the pid it prints.
func onOwnConsole(t *testing.T, bin, mode string, env ...string) int {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(append(os.Environ(), helperEnv+"="+mode), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE, HideWindow: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	var pid int
	if _, err := fmt.Fscan(out, &pid); err != nil {
		t.Fatalf("no pid from %s: %v", filepath.Base(bin), err)
	}
	return pid
}

func TestTerminateAsksASessionAloneOnItsConsoleToStop(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "stopped")
	pid := onOwnConsole(t, copyAs(t, "claude"), "breakable", "CLAUDES_PROCS_MARKER="+marker)
	start := time.Now()
	if err := Terminate(pid, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the session was killed without Ctrl-Break: %v", err)
	}
	if took := time.Since(start); took > breakGrace {
		t.Errorf("stopping took %s", took)
	}
}

func TestTerminateKillsASessionThatSharesItsConsole(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "stopped")
	claude := copyAs(t, "claude")
	pid := onOwnConsole(t, copyAs(t, "launcher"), "launch",
		"CLAUDES_PROCS_CHILD="+claude, "CLAUDES_PROCS_MARKER="+marker)
	t.Cleanup(func() {
		if p, err := os.FindProcess(pid); err == nil {
			p.Kill()
		}
	})
	if err := Terminate(pid, time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if Alive(pid) {
		t.Fatal("the session still runs")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("Ctrl-Break went to a console another program shares")
	}
}
