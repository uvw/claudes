// Package procs stops interactive Claude Code sessions, which have no job id
// and so cannot be stopped through the claude command line.
package procs

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// isClaudeExecutable reports whether file is the native claude binary: a file
// named claude, or a version under a claude/versions directory, where the native
// installer keeps them. The Claude desktop app is also named Claude, so its
// install directories are ruled out.
func isClaudeExecutable(file string) bool {
	file = strings.ToLower(strings.ReplaceAll(file, `\`, "/"))
	if strings.Contains(file, "/anthropicclaude/") || strings.Contains(file, ".app/contents/macos/") {
		return false
	}
	base := strings.TrimSuffix(path.Base(file), ".exe")
	return base == "claude" || strings.Contains(file, "/claude/versions/")
}

// isClaudeCommand reports whether a command line runs Claude Code: the native
// binary, or the npm package under node.
func isClaudeCommand(command string) bool {
	if strings.Contains(strings.ReplaceAll(command, `\`, "/"), "@anthropic-ai/claude-code") {
		return true
	}
	command = strings.TrimSpace(command)
	first, _, _ := strings.Cut(command, " ")
	if isClaudeExecutable(first) {
		return true
	}
	// ps joins the arguments with spaces, so a path with a space in it shows
	// as several words. A longer prefix counts only when it is a file that
	// exists, so an argument that ends in claude never does.
	for i := len(first) + 1; i <= len(command); i++ {
		if i < len(command) && command[i] != ' ' {
			continue
		}
		if prefix := command[:i]; isClaudeExecutable(prefix) && isFile(prefix) {
			return true
		}
	}
	return false
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// startSlack is how much later than the session's startedAt its process may
// seem to start. A session's process always starts first: Claude Code records
// startedAt once it is up, about two seconds later, and a background session
// runs in a spare process started ahead of time. The slack only absorbs
// rounding and clock adjustments.
const startSlack = 2 * time.Second

// listSlack is how much later than listedAt a process that started before it
// may seem to start. Linux gives a start time on the boot clock, which claudes
// turns into wall time with an offset it reads once. NTP slews the wall clock
// against the boot clock by up to 500 ppm, so the offset drifts by up to 0.5 ms
// a second after that; 100 ms covers over three minutes of it. macOS and
// Windows read a start time at or before the real one.
const listSlack = 100 * time.Millisecond

// checkStart refuses a process whose PID was reused, so it is not the
// session's: one that started after the session did (sessionStarted, from
// claude agents --json), or after claudes listed the sessions (listedAt). A
// zero time skips its check.
func checkStart(pid int, started, sessionStarted, listedAt time.Time) error {
	if !sessionStarted.IsZero() && started.After(sessionStarted.Add(startSlack)) {
		return fmt.Errorf("pid %d belongs to a process started %s after the session did, not to the session",
			pid, started.Sub(sessionStarted).Round(time.Second))
	}
	if !listedAt.IsZero() && started.After(listedAt.Add(listSlack)) {
		return fmt.Errorf("pid %d belongs to a process started after the sessions were listed, not to the session", pid)
	}
	return nil
}

// Running reports whether the process that started at started still runs. A
// zero started only checks the PID. A PID that a newer process took over
// does not count.
func Running(pid int, started time.Time) bool {
	now, ok := Started(pid)
	if !ok {
		return false
	}
	return started.IsZero() || now.Equal(started)
}

// versionDir matches the native installer's layout, where each Claude Code
// version is a file named for it under a versions directory, such as
// ~/.local/share/claude/versions/2.1.326.
var versionDir = regexp.MustCompile(`(?i)[/\\]claude[/\\]versions[/\\](\d+\.\d+\.\d+)(\.exe)?$`)

// VersionOf is the Claude Code version a native-installer executable path
// names, or "" for any other path, such as node running the npm package.
func VersionOf(executable string) string {
	if m := versionDir.FindStringSubmatch(executable); m != nil {
		return m[1]
	}
	return ""
}

// RunningVersion is the Claude Code version the process pid runs, read from
// its executable's path, or "" when that path does not say.
func RunningVersion(pid int) string {
	exe, err := Executable(pid)
	if err != nil {
		return ""
	}
	return VersionOf(exe)
}
