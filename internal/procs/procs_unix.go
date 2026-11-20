//go:build unix

package procs

import (
	"errors"
	"fmt"
	"os"
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
// that is not claude, or that started after the session or after listedAt
// (checkStart). A process that is already gone is not an error.
func Terminate(pid int, sessionStarted, listedAt time.Time) error {
	command, started, err := lookup(pid)
	if err != nil || command == "" {
		return err
	}
	if !isClaudeCommand(command) {
		return fmt.Errorf("pid %d is not a claude process: %.60s", pid, command)
	}
	if err := checkStart(pid, started, sessionStarted, listedAt); err != nil {
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
// signaled. Only a failure to stop root is an error.
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

// same reports whether pid is still the process that started at started. Its
// command line can be empty, as it is for a child in the middle of exec.
func same(pid int, started time.Time) bool {
	_, now, err := lookup(pid)
	return err == nil && !now.IsZero() && now.Equal(started)
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
