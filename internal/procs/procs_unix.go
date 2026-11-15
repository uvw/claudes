//go:build unix

package procs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	grace = 10 * time.Second
	poll  = 250 * time.Millisecond
)

// Terminate sends SIGTERM to a claude process, then SIGKILL after a grace
// period, and then does the same to every process it started that outlived
// it, such as shells and MCP servers, as Windows does. It refuses a process
// that is not claude, or that started after listedAt. A process that is
// already gone is not an error.
func Terminate(pid int, listedAt time.Time) error {
	command, started, err := lookup(pid)
	if err != nil || command == "" {
		return err
	}
	if !isClaudeCommand(command) {
		return fmt.Errorf("pid %d is not a claude process: %.60s", pid, command)
	}
	if err := checkStart(pid, started, listedAt); err != nil {
		return err
	}
	// Taken first: once claude exits, its children belong to init. claudes
	// itself may be one of them, when it stops the session it runs in, and is
	// left out so it can finish.
	tree, err := descendants(pid)
	if err != nil {
		tree = map[int]time.Time{}
	}
	delete(tree, os.Getpid())
	tree[pid] = started
	return stopAll(pid, tree)
}

// stopAll sends SIGTERM to every process in tree at once, waits up to the
// grace period for them all, then sends SIGKILL to those left. Each signal
// goes only to a process with its listed start time, so a reused PID is never
// signalled. Only a failure to stop root is an error.
func stopAll(root int, tree map[int]time.Time) error {
	var rootErr error
	for p, started := range tree {
		if !same(p, started) {
			delete(tree, p)
			continue
		}
		if err := signal(p, syscall.SIGTERM); err != nil && p == root {
			rootErr = err
		}
	}
	if rootErr != nil {
		return rootErr
	}
	for range int(grace / poll) {
		for p := range tree {
			if err := syscall.Kill(p, 0); errors.Is(err, syscall.ESRCH) {
				delete(tree, p)
			}
		}
		if len(tree) == 0 {
			return nil
		}
		time.Sleep(poll)
	}
	for p, started := range tree {
		if same(p, started) {
			if err := signal(p, syscall.SIGKILL); err != nil && p == root {
				return err
			}
		}
	}
	return nil
}

// Started is when the process with this PID started, if it runs.
func Started(pid int) (time.Time, bool) {
	command, started, err := lookup(pid)
	return started, err == nil && command != ""
}

// Alive reports whether a process with this PID is still running.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// same reports whether pid is still the process that started at started.
func same(pid int, started time.Time) bool {
	command, now, err := lookup(pid)
	return err == nil && command != "" && now.Equal(started)
}

// descendants maps every process pid started, directly or not, to its start
// time.
func descendants(pid int) (map[int]time.Time, error) {
	cmd := exec.Command("ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list processes with ps: %w", err)
	}
	children := map[int][]int{}
	starts := map[int]time.Time{}
	for line := range strings.Lines(string(out)) {
		fields := strings.Fields(line)
		if len(fields) != 7 {
			continue
		}
		p, err1 := strconv.Atoi(fields[0])
		parent, err2 := strconv.Atoi(fields[1])
		started, err3 := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(fields[2:], " "), time.Local)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		children[parent] = append(children[parent], p)
		starts[p] = started
	}
	found := map[int]time.Time{}
	queue := []int{pid}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p] {
			if _, seen := found[c]; !seen && c != pid {
				found[c] = starts[c]
				queue = append(queue, c)
			}
		}
	}
	return found, nil
}

// lookup returns the process's command line and start time, or "" when no
// process has that PID.
func lookup(pid int) (string, time.Time, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=", "-o", "command=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	line := strings.TrimSpace(string(out))
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && line == "" {
		// ps exits 1 with no output when no process has the PID.
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cannot look up pid %d with ps: %w", pid, err)
	}
	return parsePS(pid, line)
}

// parsePS splits "Sat Nov 14 10:00:00 2026 /path/claude --flag" into the
// command line and its start time.
func parsePS(pid int, line string) (string, time.Time, error) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return "", time.Time{}, fmt.Errorf("cannot read ps output for pid %d: %q", pid, line)
	}
	started, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(fields[:5], " "), time.Local)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cannot read the start time of pid %d: %w", pid, err)
	}
	return strings.Join(fields[5:], " "), started, nil
}

func signal(pid int, sig syscall.Signal) error {
	err := syscall.Kill(pid, sig)
	switch {
	case err == nil, errors.Is(err, syscall.ESRCH):
		return nil
	case errors.Is(err, syscall.EPERM):
		return fmt.Errorf("cannot stop pid %d: %w", pid, err)
	default:
		return err
	}
}
