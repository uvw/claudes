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
// named claude, or a version under a claude/versions folder, where the native
// installer keeps them. The Claude desktop app is also named Claude, so its
// install folders are ruled out.
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
	if strings.Contains(command, "@anthropic-ai/claude-code") {
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

// checkStart refuses a process that started after claudes listed the
// sessions: its PID was reused, so it is not the session's process.
func checkStart(pid int, started, listedAt time.Time) error {
	if !listedAt.IsZero() && started.After(listedAt) {
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
// version is a file named for it under a versions folder, such as
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
