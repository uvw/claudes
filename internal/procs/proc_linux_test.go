package procs

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestParseStatCountsFromTheLastParenthesis(t *testing.T) {
	line := "4242 (weird) name (x)) S 77 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 3 0 123456 1000 10 0 0\n"
	parent, ticks, err := parseStat([]byte(line))
	if err != nil || parent != 77 || ticks != 123456 {
		t.Fatalf("parent %d, ticks %d, err %v", parent, ticks, err)
	}
	for _, bad := range []string{
		"4242 no name", "4242 (x) S 77", "4242 (x) S ppid 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19",
		"4242 (x) S 77 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 -5",
		"4242 (x) S 77 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 99999999999999999999",
	} {
		if _, _, err := parseStat([]byte(bad)); err == nil {
			t.Errorf("read %q", bad)
		}
	}
}

func TestGoneCountsAProcessThatExitedMidRead(t *testing.T) {
	for _, err := range []error{
		&fs.PathError{Op: "open", Path: "/proc/4242/stat", Err: syscall.ENOENT},
		&fs.PathError{Op: "read", Path: "/proc/4242/cmdline", Err: syscall.ESRCH},
	} {
		if !gone(err) {
			t.Errorf("%v: not gone", err)
		}
	}
	if gone(&fs.PathError{Op: "open", Path: "/proc/4242/stat", Err: syscall.EACCES}) {
		t.Error("a process claudes may not read counts as gone")
	}
}

func TestRunningVersionOfAPrunedVersion(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "claude", "versions", "2.1.300")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), helperEnv+"=sleep")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	if err := os.Remove(bin); err != nil {
		t.Fatal(err)
	}
	if got := RunningVersion(cmd.Process.Pid); got != "2.1.300" {
		t.Fatalf("version %q, want 2.1.300", got)
	}
}

func TestLookupReadsThisProcessFromProc(t *testing.T) {
	command, started, err := lookup(os.Getpid())
	if err != nil || command == "" {
		t.Fatalf("command %q, err %v", command, err)
	}
	if age := time.Since(started); age < 0 || age > time.Hour {
		t.Fatalf("started %s ago", age)
	}
	again, _, _ := lookup(os.Getpid())
	_, twice, _ := lookup(os.Getpid())
	if again != command || !twice.Equal(started) {
		t.Fatalf("two lookups disagree: %v and %v", started, twice)
	}
}
