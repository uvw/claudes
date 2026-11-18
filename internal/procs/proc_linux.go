package procs

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// clockTicks is how many ticks a second /proc counts process start times in.
// It is USER_HZ, which the kernel fixes at 100 for /proc on every
// architecture Go supports.
const clockTicks = 100

// bootTime is when the system booted, worked out once from /proc/uptime, so
// every start time claudes computes in one run uses the same base and two
// lookups of one process agree exactly.
var bootTime = sync.OnceValues(func() (time.Time, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return time.Time{}, err
	}
	up, _, _ := strings.Cut(string(data), " ")
	seconds, err := strconv.ParseFloat(up, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot read /proc/uptime: %q", data)
	}
	return time.Now().Add(-time.Duration(seconds * float64(time.Second))).Truncate(10 * time.Millisecond), nil
})

// lookup returns the process's command line and start time, or "" when no
// process has that PID, from /proc. Its start time is exact to the clock
// tick, where ps gives whole seconds.
func lookup(pid int) (string, time.Time, error) {
	_, started, err := stat(pid)
	if errors.Is(err, fs.ErrNotExist) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("cannot read the command line of pid %d: %w", pid, err)
	}
	// The arguments end in NUL bytes. Joined with spaces, they read as ps
	// prints them; a zombie has none, and counts as gone.
	return strings.TrimSpace(string(bytes.ReplaceAll(data, []byte{0}, []byte{' '}))), started, nil
}

// stat reads a process's parent and start time from /proc/PID/stat.
func stat(pid int) (parent int, started time.Time, err error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, time.Time{}, err
	}
	parent, ticks, err := parseStat(data)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("cannot read /proc/%d/stat: %w", pid, err)
	}
	boot, err := bootTime()
	if err != nil {
		return 0, time.Time{}, err
	}
	return parent, boot.Add(time.Duration(ticks) * time.Second / clockTicks), nil
}

// parseStat reads the parent PID and the start time in clock ticks from a
// /proc/PID/stat line. The program's name, in parentheses second, may hold
// spaces and parentheses itself, so the fields are counted from the last ")".
func parseStat(data []byte) (parent int, ticks uint64, err error) {
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return 0, 0, fmt.Errorf("no program name in %q", data)
	}
	// After the name: state, ppid, and on to starttime, the 22nd field of
	// the line and the 20th after the name.
	fields := strings.Fields(string(data[i+1:]))
	if len(fields) < 20 {
		return 0, 0, fmt.Errorf("too few fields in %q", data)
	}
	if parent, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, err
	}
	if ticks, err = strconv.ParseUint(fields[19], 10, 64); err != nil {
		return 0, 0, err
	}
	return parent, ticks, nil
}

// descendants maps every process pid started, directly or not, to its start
// time.
func descendants(pid int) (map[int]time.Time, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("cannot list processes in /proc: %w", err)
	}
	children := map[int][]int{}
	starts := map[int]time.Time{}
	for _, e := range entries {
		p, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		parent, started, err := stat(p)
		if err != nil {
			// It exited while the list was read.
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
