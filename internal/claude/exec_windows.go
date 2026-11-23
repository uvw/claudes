//go:build windows

package claude

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Exec runs the named binary in this terminal and exits with its exit code.
// Windows cannot replace a running process, so claudes waits for the child
// instead. It returns only when the binary cannot be started.
func Exec(binary string, args ...string) error {
	path, err := exec.LookPath(binary)
	if err != nil {
		return err
	}
	if err := checkArgs(path, args); err != nil {
		return err
	}
	cmd := exec.Command(path, args...)
	cmd.Env = childEnv()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl-C and Ctrl-Break reach every process on the console; the child
	// decides what they mean, and claudes waits for it to exit. The handler
	// must be a channel: with signal.Ignore, Go declines the console event and
	// Windows ends claudes at once, orphaning the child.
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	if err := cmd.Start(); err != nil {
		return err
	}
	endWithThisProcess(cmd.Process.Pid)
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		return err
	}
	os.Exit(0)
	return nil
}

// endWithThisProcess ends the process pid when claudes ends, as on Unix, where
// the child replaces claudes: killing claudes would otherwise leave the child
// reading the console beside the shell. pid joins a job that Windows closes
// with claudes' last handle. What pid starts breaks away from the job, so the
// background service claude agents starts outlives both. A failure leaves the
// child running as before.
func endWithThisProcess(pid int) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if windows.AssignProcessToJobObject(job, h) != nil {
		windows.CloseHandle(job)
	}
	// The job's handle stays open until claudes exits, which closes the job.
}
