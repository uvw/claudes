package claude

import (
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/shell"
)

// configDirVar picks the Claude Code directory, and so the account, a claude
// process runs under.
const configDirVar = "CLAUDE_CONFIG_DIR"

// ConfigDir is the CLAUDE_CONFIG_DIR a claude call runs with, which picks the
// account and the daemon it reaches. The zero value leaves claudes' own
// environment as it is, for the account claudes runs as.
//
// Claude Code names its keychain entry after the variable's exact value, and
// with the variable set looks for .claude.json inside the directory rather
// than beside it. So ~/.claude is reached with the variable unset, never
// spelled out, and a profile with the exact path its own tool sets.
type ConfigDir struct {
	// Path is the value to set, or "" with Unset.
	Path string
	// Unset runs claude without the variable, for ~/.claude when claudes
	// itself runs with one.
	Unset bool
}

// Own reports whether d is the zero value: claudes' own environment.
func (d ConfigDir) Own() bool { return d == ConfigDir{} }

// String is d as a log shows it.
func (d ConfigDir) String() string {
	switch {
	case d.Unset:
		return "unset"
	case d.Path == "":
		return "inherited"
	}
	return d.Path
}

// ResumeCommand is shell.ResumeCommand under d: one that sets or unsets
// CLAUDE_CONFIG_DIR first, unless d is claudes' own.
func (d ConfigDir) ResumeCommand(sessionID, cwd string, interactive bool) string {
	if d.Own() {
		return shell.ResumeCommand(sessionID, cwd, interactive)
	}
	return shell.ResumeCommandIn(d.Path, sessionID, cwd, interactive)
}

// apply is env with d's CLAUDE_CONFIG_DIR.
func (d ConfigDir) apply(env []string) []string {
	if d.Own() {
		return env
	}
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		return strings.EqualFold(name, configDirVar)
	})
	if d.Path != "" {
		out = append(out, configDirVar+"="+d.Path)
	}
	return out
}
