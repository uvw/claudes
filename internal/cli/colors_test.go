package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

var sgr = regexp.MustCompile("\x1b\\[[0-9;]*m")

func (h *harness) env(vars ...string) {
	h.app.Environ = func() []string { return vars }
}

func assertPlain(t *testing.T, what, text string) {
	t.Helper()
	if strings.Contains(text, "\x1b") {
		t.Fatalf("%s has colors:\n%q", what, text)
	}
}

// lsWorld has a session of each kind that ls colors.
func lsWorld(t *testing.T, h *harness) {
	t.Helper()
	pinned, old := h.bg("pinned-one", "busy"), h.bg("old-one", "idle")
	h.agents.snapshots = [][]claude.Session{{pinned, h.bg("asking", "waiting"), h.tty("prompt", "shell"), old}}
	h.pin(t, pinned)
	writeState(t, h, old.JobID, "2.1.322")
}

func TestLsTextIsColoredWithCLICOLORFORCEAndLinesUpAsWithout(t *testing.T) {
	h := newHarness(t)
	lsWorld(t, h)
	h.ok(t, "ls", "-o", "text")
	plain := h.out.String()
	assertPlain(t, "ls without CLICOLOR_FORCE", plain)

	h.out.Reset()
	h.env("CLICOLOR_FORCE=1")
	h.ok(t, "ls", "-o", "text")

	out := h.out.String()
	for _, want := range []string{
		h.app.colors.Blue("yes"), h.app.colors.Green("busy"), h.app.colors.Magenta("waiting"),
		h.app.colors.Cyan("shell"), h.app.colors.Yellow("2.1.322 (old)"),
	} {
		assertContains(t, out, want)
	}
	if !strings.Contains(out, "\x1b[2m") {
		t.Fatalf("CWD is not faint:\n%q", out)
	}
	if strings.Contains(out, h.app.colors.Green("idle")) || !h.app.colors.Colors() {
		t.Fatalf("idle is colored, or colors are off:\n%q", out)
	}
	if got := sgr.ReplaceAllString(out, ""); got != plain {
		t.Fatalf("colored ls without its colors:\n%s\nwant:\n%s", got, plain)
	}
}

// The harness writes to buffers, so each case is a pipe.
func TestColorVariablesOnAPipe(t *testing.T) {
	for _, tc := range []struct {
		env     []string
		colored bool
	}{
		{[]string{"CLICOLOR=1"}, false},
		{[]string{"CLICOLOR_FORCE=1"}, true},
		{[]string{"CLICOLOR_FORCE=0"}, false},
		{[]string{"CLICOLOR_FORCE="}, false},
		{[]string{"CLICOLOR=0", "CLICOLOR_FORCE=1"}, true},
		{[]string{"CLICOLOR_FORCE=1", "NO_COLOR=1"}, false},
		{[]string{"CLICOLOR=1", "NO_COLOR=1"}, false},
		{[]string{"FORCE_COLOR=1"}, true},
		{[]string{"FORCE_COLOR=0"}, false},
		{[]string{"CLICOLOR=0", "FORCE_COLOR=1"}, true},
		{[]string{"FORCE_COLOR=1", "NO_COLOR=1"}, false},
	} {
		t.Run(strings.Join(tc.env, " "), func(t *testing.T) {
			h := newHarness(t)
			lsWorld(t, h)
			h.env(tc.env...)
			h.ok(t, "ls")
			out := h.out.String()
			h.exits(t, exitNoMatch, "where", "nobody")
			errOut := h.err.String()
			if !tc.colored {
				assertPlain(t, "stdout", out)
				assertPlain(t, "stderr", errOut)
				return
			}
			assertContains(t, out, h.app.colors.Green("busy"))
			if !strings.HasPrefix(errOut, "\x1b[31mclaudes: ") {
				t.Fatalf("the error is not red:\n%q", errOut)
			}
		})
	}
}

func TestJSONYAMLCompletionInitAndSkillAreNeverColored(t *testing.T) {
	for _, args := range [][]string{
		{"ls", "-o", "json"}, {"ls", "--json"}, {"ls", "-o", "yaml"}, {"where", "pinned-one", "asking", "-o", "json"},
		{"doctor", "-o", "yaml"}, {"session", "ls", "-o", "json"}, {"restart", "--dry-run", "-o", "json"},
		{"__complete", "ls", ""}, {"completion", "bash"}, {"init", "zsh"}, {"skill"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t)
			lsWorld(t, h)
			h.env("CLICOLOR_FORCE=1")
			h.run(args...)
			assertPlain(t, "stdout", h.out.String())
		})
	}
}

func TestProgressFailuresAndErrorsAreColoredAndKeepTheirColumn(t *testing.T) {
	h := newHarness(t)
	bad, good := stopped(h.bg("bad", "")), stopped(h.bg("good", ""))
	h.agents.finished = []claude.Session{bad, good}
	h.agents.failures = map[string]string{"rm " + bad.JobID: "worktree has changes"}
	h.env("CLICOLOR_FORCE=1")

	h.fails(t, "gc", "bad", "good")

	assertContains(t, h.out.String(), "  "+h.app.colors.Green("removed")+"    good")
	assertContains(t, h.err.String(), "  "+h.app.errColors.Red("FAILED")+"     bad")
	assertContains(t, h.err.String(), h.app.errColors.Red("1 session(s) need attention:"))
	assertContains(t, h.err.String(), h.app.errColors.Red("  bad: worktree has changes"))

	h.err.Reset()
	h.exits(t, exitNoMatch, "where", "nobody")
	if !strings.HasPrefix(h.err.String(), "\x1b[31mclaudes: ") {
		t.Fatalf("the error is not red:\n%q", h.err)
	}
}

func TestDoctorColorsEachResult(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("odd", "thinking")}}
	h.agents.version = "2.9.0"
	h.env("CLICOLOR_FORCE=1")

	h.fails(t, "doctor")

	out := h.out.String()
	for _, want := range []string{h.app.colors.Green("ok"), h.app.colors.Yellow("warn"), h.app.colors.Red("fail")} {
		assertContains(t, out, want)
	}
	assertContains(t, h.err.String(), h.app.errColors.Red("1 check(s) failed."))
}

func TestDoctorEnvShowsEachColorVariable(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("FORCE_COLOR", "3")
	t.Setenv("CLICOLOR", "0")
	h := newHarness(t)
	h.ok(t, "doctor", "env", "-o", "json")
	var report map[string]map[string]string
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	env := report["environment"]
	if env["NO_COLOR"] != `""` || env["CLICOLOR_FORCE"] != "1" || env["FORCE_COLOR"] != "3" || env["CLICOLOR"] != "0" {
		t.Fatalf("environment %v", env)
	}
}
