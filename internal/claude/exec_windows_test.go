package claude

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// running reports whether pid still runs, waiting up to wait for it to end.
func running(t *testing.T, pid int, wait time.Duration) bool {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(wait/time.Millisecond))
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}

func TestExecEndsTheChildWithClaudesButNotWhatTheChildStarted(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"=exec")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var child, grand int
	if _, err := fmt.Fscan(out, &child, &grand); err != nil {
		cmd.Process.Kill()
		t.Fatalf("no pids from the child: %v", err)
	}
	t.Cleanup(func() {
		for _, pid := range []int{child, grand} {
			if p, err := os.FindProcess(pid); err == nil {
				p.Kill()
			}
		}
	})
	if !running(t, child, 0) || !running(t, grand, 0) {
		t.Fatal("the child or its own child is not running")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	if running(t, child, 5*time.Second) {
		t.Error("the child outlived claudes")
	}
	if !running(t, grand, 0) {
		t.Error("what the child started ended with claudes, as the background service would")
	}
}
