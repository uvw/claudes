//go:build unix && !linux

package procs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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
