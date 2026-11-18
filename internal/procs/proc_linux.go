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
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// clockTicks is how many ticks a second /proc counts process start times in.
// It is USER_HZ, which the kernel fixes at 100 for /proc on every
// architecture Go supports.
const clockTicks = 100

// bootTime is when the system booted, worked out once, so two lookups of one
// process agree exactly. The kernel counts start times on CLOCK_BOOTTIME,
// rounded down to the tick. Reading that clock just after the wall clock puts
// the base at or before the real boot; listSlack covers drift after that.
var bootTime = sync.OnceValues(func() (time.Time, error) {
	now := time.Now()
	var up unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &up); err != nil {
		return time.Time{}, fmt.Errorf("cannot read the time since boot: %w", err)
	}
	// Round(0) drops the monotonic reading, which leaves out time asleep, so
	// start times compare by the wall clock alone.
	return now.Add(-time.Duration(up.Nano())).Round(0), nil
})

// gone reports whether reading a process's files under /proc failed because
// the process exited: before the open, or between the open and the read.
func gone(err error) bool { return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESRCH) }

// lookup returns the process's command line and start time, or "" when no
// process has that PID, from /proc. Its start time is exact to the clock
// tick, where ps gives whole seconds.
func lookup(pid int) (string, time.Time, error) {
	_, started, err := stat(pid)
	if gone(err) {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, err
	}
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if gone(err) {
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
	// One tick at a time, so years of uptime cannot overflow.
	return parent, boot.Add(time.Duration(ticks) * (time.Second / clockTicks)), nil
}

// parseStat reads the parent PID and the start time in clock ticks from a
// /proc/PID/stat line. The program's name, in parentheses second, may hold
// spaces and parentheses itself, so the fields are counted from the last ")".
func parseStat(data []byte) (parent int, ticks int64, err error) {
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
	if ticks, err = strconv.ParseInt(fields[19], 10, 64); err != nil {
		return 0, 0, err
	}
	if ticks < 0 {
		return 0, 0, fmt.Errorf("a negative start time in %q", data)
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
