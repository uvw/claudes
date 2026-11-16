//go:build windows

package procs

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	stillActive = 259
	exitWait    = 10 * time.Second
)

// Terminate kills a claude process and every process it started, then waits
// for it to exit. Windows has no SIGTERM for console programs, so nothing is
// asked to stop gracefully first. It refuses a process that is not claude, or
// that started after listedAt. A process that is already gone is not an error.
func Terminate(pid int, listedAt time.Time) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot open pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if exited(h) {
		return nil
	}
	image, err := imagePath(h)
	if err != nil {
		return fmt.Errorf("cannot read the program of pid %d: %w", pid, err)
	}
	if !isClaudeExecutable(image) {
		return fmt.Errorf("pid %d is not a claude process: %s", pid, image)
	}
	started, err := startTime(h)
	if err != nil {
		return fmt.Errorf("cannot read the start time of pid %d: %w", pid, err)
	}
	if err := checkStart(pid, started, listedAt); err != nil {
		return err
	}
	// claudes itself may be one of them, when it stops the session it runs
	// in, and is left out so it can finish.
	children := slices.DeleteFunc(descendants(uint32(pid), started), func(c child) bool { return int(c.pid) == os.Getpid() })
	if err := windows.TerminateProcess(h, 1); err != nil && !exited(h) {
		return fmt.Errorf("cannot stop pid %d: %w", pid, err)
	}
	for _, c := range children {
		killChild(c)
	}
	event, err := windows.WaitForSingleObject(h, uint32(exitWait/time.Millisecond))
	if err != nil || event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("pid %d did not exit within %s", pid, exitWait)
	}
	return nil
}

// Alive reports whether a process with this PID is still running.
func Alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	return !exited(h)
}

func exited(h windows.Handle) bool {
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code != stillActive
}

func imagePath(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

func startTime(h windows.Handle) (time.Time, error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, created.Nanoseconds()), nil
}

type child struct {
	pid     uint32
	started time.Time
}

// descendants lists every process below root. Windows keeps a parent's PID
// after the parent exits and may reuse it, so a process counts as a child only
// if it started after its parent.
func descendants(root uint32, rootStarted time.Time) []child {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	parents := map[uint32][]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID != entry.ParentProcessID {
			parents[entry.ParentProcessID] = append(parents[entry.ParentProcessID], entry.ProcessID)
		}
	}
	var out []child
	queue := []child{{root, rootStarted}}
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, pid := range parents[parent.pid] {
			started, ok := processStart(pid)
			if !ok || started.Before(parent.started) {
				continue
			}
			c := child{pid, started}
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}

// Started is when the process with this PID started, if it runs.
func Started(pid int) (time.Time, bool) {
	if !Alive(pid) {
		return time.Time{}, false
	}
	return processStart(uint32(pid))
}

func processStart(pid uint32) (time.Time, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return time.Time{}, false
	}
	defer windows.CloseHandle(h)
	started, err := startTime(h)
	return started, err == nil
}

// killChild kills a child if it is still the process that was listed.
func killChild(c child) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, c.pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	if started, err := startTime(h); err == nil && started.Equal(c.started) {
		// A child that exited meanwhile needs nothing more.
		_ = windows.TerminateProcess(h, 1)
	}
}
