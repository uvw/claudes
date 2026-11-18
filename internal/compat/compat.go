// Package compat checks that the installed Claude Code still looks the way
// claudes expects. claudes reads `claude agents --json` and files under
// ~/.claude/jobs, and neither is a stable Claude Code interface.
package compat

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/jobs"
	"github.com/uvw/claudes/internal/version"
)

// Tested is the range of Claude Code versions claudes was checked against.
// A newer patch release inside the same minor version is expected to work;
// the shape checks below catch it when it does not.
var Tested = Range{Min: "2.1.325", Max: "2.1.325"}

// Range is a span of Claude Code versions.
type Range struct{ Min, Max string }

// String is the range as a reader sees it, such as "2.1.325 to 2.1.x".
func (r Range) String() string { return r.Min + " to " + version.Minor(r.Max) + ".x" }

// CheckVersion returns a warning when installed is outside the tested range:
// older than Min, or in a newer minor or major version than Max.
func (r Range) CheckVersion(installed string) string {
	switch {
	case installed == "":
		return ""
	case version.Less(installed, r.Min):
		return fmt.Sprintf("Claude Code %s is older than claudes was tested with (%s)", installed, r)
	case version.Compare(version.Minor(installed), version.Minor(r.Max)) > 0:
		return fmt.Sprintf("Claude Code %s is newer than claudes was tested with (%s)", installed, r)
	}
	return ""
}

// kinds and statuses are the values of `claude agents --json` that claudes
// makes decisions on. Status decides whether a session is mid-turn and gets
// pinged; kind decides how it is stopped. An unknown state only shows in ls,
// and gc treats it as not finished, so states are not checked.
var (
	kinds    = []string{claude.KindBackground, claude.KindInteractive}
	statuses = []string{"", "idle", "busy", "waiting", "shell"}
)

// CheckSessions lists what in sessions claudes does not recognize, one line
// per problem.
func CheckSessions(sessions []claude.Session) []string {
	var problems []string
	for _, s := range sessions {
		who := s.Label()
		if !slices.Contains(kinds, s.Kind) {
			problems = append(problems, fmt.Sprintf("%s has kind %q, not one of %s", who, s.Kind, listOf(kinds)))
		}
		if s.Running() && s.Status == "" {
			problems = append(problems, fmt.Sprintf("%s is running but has no status, so claudes cannot tell whether it is mid-turn", who))
		} else if !slices.Contains(statuses, s.Status) {
			problems = append(problems, fmt.Sprintf("%s has status %q, not one of %s", who, s.Status, listOf(statuses)))
		}
		if !claude.ValidSessionID(s.SessionID) {
			problems = append(problems, fmt.Sprintf("%s has session id %q, not a UUID", who, s.SessionID))
		}
		if s.JobID != "" && !jobs.ValidID(s.JobID) {
			problems = append(problems, fmt.Sprintf("%s has job id %q, not eight hex digits", who, s.JobID))
		}
	}
	return problems
}

// CheckPins lists the entries of pins.json that are not job ids.
func CheckPins(ids []string) []string {
	var problems []string
	for _, id := range ids {
		if !jobs.ValidID(id) {
			problems = append(problems, fmt.Sprintf("pins.json holds %q, not a job id", id))
		}
	}
	return problems
}

// commands and flags are the parts of claude's command line that claudes
// calls. CheckHelp looks for them in `claude --help`.
var (
	commands    = []string{"agents", "attach", "logs", "plugin", "respawn", "rm", "stop"}
	flags       = []string{"--bg", "--resume", "--version"}
	agentsFlags = []string{"--json", "--all"}
)

var commandLine = regexp.MustCompile(`^  ([a-z][a-z|-]*)`)

// CheckHelp lists the commands and flags claudes uses that are missing from
// the output of `claude --help` (root) and `claude agents --help` (agents).
func CheckHelp(root, agents string) []string {
	have := map[string]bool{}
	inCommands := false
	for line := range strings.Lines(root) {
		if strings.HasPrefix(line, "Commands:") {
			inCommands = true
			continue
		}
		if m := commandLine.FindStringSubmatch(line); inCommands && m != nil {
			for name := range strings.SplitSeq(m[1], "|") {
				have[name] = true
			}
		}
	}
	var missing []string
	for _, c := range commands {
		if !have[c] {
			missing = append(missing, "claude "+c)
		}
	}
	for _, f := range flags {
		if !hasFlag(root, f) {
			missing = append(missing, "claude "+f)
		}
	}
	for _, f := range agentsFlags {
		if !hasFlag(agents, f) {
			missing = append(missing, "claude agents "+f)
		}
	}
	return missing
}

// hasFlag finds flag as an option name in help text, not inside a sentence:
// it must start a line's option list or follow a comma there.
func hasFlag(help, flag string) bool {
	re := regexp.MustCompile(`(?m)^\s+(?:-\w,\s+)?(?:--[\w-]+,\s+)*` + regexp.QuoteMeta(flag) + `(?:[,\s]|$)`)
	return re.MatchString(help)
}

// listOf joins the non-empty values for a message.
func listOf(values []string) string {
	return strings.Join(slices.DeleteFunc(slices.Clone(values), func(v string) bool { return v == "" }), ", ")
}
