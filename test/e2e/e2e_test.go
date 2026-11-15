package e2e

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/test/e2e/internal/fake"
)

const restartPing = "Claude Code was restarted while you were working. Pick up where you left off."

func TestLsJSONPrintsEveryKeyInCamelCase(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "busy")
	w.tty("console")
	w.pin(alpha)
	w.jobVersion(alpha, "2.1.300")

	r := w.ok("ls", "-o", "json")

	var rows []map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &rows); err != nil {
		t.Fatalf("ls -o json printed something other than a list: %v\n%s", err, r.stdout)
	}
	want := []string{"cwd", "id", "kind", "name", "outdated", "pid", "pinned", "sessionId", "startedAt", "state", "status", "teammates", "version"}
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, want) {
			t.Fatalf("keys %q, want %q", keys, want)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	first := rows[0]
	if first["name"] != "alpha" || first["id"] != alpha.ID || first["sessionId"] != alpha.SessionID ||
		first["pinned"] != true || first["version"] != "2.1.300" || first["outdated"] != true ||
		first["pid"] != float64(alpha.PID) || first["startedAt"] != float64(alpha.StartedAt) {
		t.Fatalf("pinned row %v", first)
	}
	if tty := rows[1]; tty["id"] != "" || tty["kind"] != "interactive" || tty["pinned"] != false || tty["version"] != "" {
		t.Fatalf("interactive row %v", tty)
	}
}

func TestLsAndWhereMatchWordsAndPatterns(t *testing.T) {
	w := newWorld(t)
	one := w.bg("plan-one", "idle")
	w.bg("plan-two", "idle")
	w.bg("replan", "idle")
	w.bg("other", "idle")

	names := func(r result) []string {
		var rows []struct{ Name string }
		if err := json.Unmarshal([]byte(r.stdout), &rows); err != nil {
			t.Fatalf("%v\n%s", err, r.stdout)
		}
		var out []string
		for _, row := range rows {
			out = append(out, row.Name)
		}
		slices.Sort(out)
		return out
	}
	if got := names(w.ok("ls", "plan", "--json")); !slices.Equal(got, []string{"plan-one", "plan-two", "replan"}) {
		t.Fatalf("ls plan: %q", got)
	}
	if got := names(w.ok("ls", "plan*", "--json")); !slices.Equal(got, []string{"plan-one", "plan-two"}) {
		t.Fatalf("ls plan*: %q", got)
	}
	if got := names(w.ok("where", "PLAN-T*", "-o", "json")); !slices.Equal(got, []string{"plan-two"}) {
		t.Fatalf("where PLAN-T*: %q", got)
	}

	if r := w.ok("where", "plan-one"); r.stdout != one.Cwd+"\n" {
		t.Fatalf("where plan-one printed %q, want the bare folder %q", r.stdout, one.Cwd)
	}
	if r := w.ok("where", one.ID[:5]); r.stdout != one.Cwd+"\n" {
		t.Fatalf("where by id prefix printed %q", r.stdout)
	}
	r := w.ok("where", "plan*")
	if lines := strings.Split(strings.TrimSpace(r.stdout), "\n"); len(lines) != 2 || !strings.HasPrefix(lines[0], "plan-") {
		t.Fatalf("where plan* printed:\n%s", r.stdout)
	}
	r = w.ok("ls", "-o", "yaml", "other")
	assertContains(t, r.stdout, "sessionId: "+sessionID("other"))

	r = w.run("where", "nothing-like-it")
	if r.code != 2 || !strings.Contains(r.stderr, "nothing-like-it") || r.stdout != "" {
		t.Fatalf("an unmatched target: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
	if r := w.run("ls", "-o", "xml"); r.code != 1 || !strings.Contains(r.stderr, "must be text, json, or yaml") {
		t.Fatalf("-o xml: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestLsIsColoredOnlyWithCLICOLORWhenStdoutIsAPipe(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "busy")

	if r := w.ok("ls"); strings.Contains(r.stdout, "\x1b") {
		t.Fatalf("ls to a pipe has colors:\n%q", r.stdout)
	}
	w.env = append(w.env, "CLICOLOR=1")
	if r := w.ok("ls"); !strings.Contains(r.stdout, "\x1b[32mbusy\x1b[m") {
		t.Fatalf("ls with CLICOLOR=1 has no colors:\n%q", r.stdout)
	}
	w.env = append(w.env, "NO_COLOR=1")
	if r := w.ok("ls"); strings.Contains(r.stdout, "\x1b") {
		t.Fatalf("ls with NO_COLOR has colors:\n%q", r.stdout)
	}
}

func TestPinAndUnpinWriteWhatAgentViewWrites(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "idle")
	beta := w.bg("beta", "idle")
	w.tty("console")
	pins := filepath.Join(w.jobsDir(), "pins.json")

	r := w.ok("pin", "alpha", beta.SessionID)
	assertContains(t, r.stdout, "pinned alpha ("+alpha.ID+")")
	want := "[\n  \"" + alpha.ID + "\",\n  \"" + beta.ID + "\"\n]"
	if data, _ := os.ReadFile(pins); string(data) != want {
		t.Fatalf("pins.json\n got: %q\nwant: %q", data, want)
	}
	if r := w.ok("pin", "alpha"); !strings.Contains(r.stdout, "already pinned: alpha") {
		t.Fatalf("pinning again printed %q", r.stdout)
	}

	order := filepath.Join(w.jobsDir(), alpha.ID, "order")
	w.writeFile(order, "0")
	w.ok("unpin", "alpha")
	if _, err := os.Stat(order); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unpin left jobs/%s/order: %v", alpha.ID, err)
	}
	if data, _ := os.ReadFile(pins); string(data) != "[\n  \""+beta.ID+"\"\n]" {
		t.Fatalf("pins.json after unpin %q", data)
	}
	if entries, _ := os.ReadDir(w.jobsDir()); slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return strings.Contains(e.Name(), ".lock") || strings.HasPrefix(e.Name(), "pins.json.")
	}) {
		t.Fatalf("lock or temporary file left in jobs: %v", entries)
	}

	if r := w.run("pin", "console"); r.code != 4 || !strings.Contains(r.stderr, "only background sessions can be pinned") {
		t.Fatalf("pinning an interactive session: exit %d, stderr %q", r.code, r.stderr)
	}
	if len(w.actions()) != 0 {
		t.Fatalf("pin called claude: %q", argsOf(w.actions()))
	}
}

func TestRestartRespawnsByJobID(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "idle")
	beta := w.bg("beta", "idle")

	r := w.ok("restart", beta.SessionID[:8])

	assertContains(t, r.stdout, "restarted  beta")
	assertArgs(t, w.actions(), []string{"respawn", beta.ID})
	if s := w.state()["beta"]; s.PID == beta.PID || s.PID == 0 {
		t.Fatalf("beta pid %d after respawn", s.PID)
	}
}

func TestRestartPingStopsAndResumesInTheSessionsFolder(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "busy")
	// Quotes, a backslash before a quote, and shell operators must reach
	// claude as one argument, unchanged.
	prompt := `carry on: "quoted" \"escaped\" it's $HOME & | ; %PATH% \`

	r := w.ok("restart", "alpha", "--ping-text", prompt)

	assertContains(t, r.stdout, "restarted  alpha and pinged")
	got := w.actions()
	assertArgs(t, got, []string{"stop", alpha.ID}, []string{"--bg", "--resume", alpha.SessionID, prompt})
	assertIn(t, got[1], alpha.Cwd)
	if s := w.state()["alpha"]; s.PID == 0 {
		t.Fatal("alpha is not running after restart --ping")
	}
	if _, err := os.Stat(w.parkPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("park file left after a resume that worked: %v", err)
	}
}

func TestBulkRestartLeavesPinnedAndInteractiveAloneAndDoesItsOwnSessionLast(t *testing.T) {
	w := newWorld(t)
	host := w.bg("host", "busy")
	alpha := w.bg("alpha", "busy")
	beta := w.bg("beta", "idle")
	gamma := w.bg("gamma", "busy")
	console := w.tty("console")
	w.finished("old", "done", time.Hour)
	w.pin(gamma)
	w.env = append(w.env, "CLAUDE_CODE_SESSION_ID="+host.SessionID)

	r := w.ok("restart", "--ping")

	got := w.actions()
	assertArgs(t, got,
		[]string{"stop", alpha.ID},
		[]string{"--bg", "--resume", alpha.SessionID, restartPing},
		[]string{"respawn", beta.ID},
		[]string{"respawn", host.ID},
	)
	assertIn(t, got[1], alpha.Cwd)
	assertContains(t, r.stdout, "Pinned, not restarted: gamma")
	assertContains(t, r.stdout, "# console")
	assertContains(t, r.stdout, "--resume "+console.SessionID)
	if !strings.HasSuffix(strings.TrimSpace(r.stdout), "Restarting this session (host) last. Output ends here.") {
		t.Fatalf("the host was not last:\n%s", r.stdout)
	}
}

func TestRestartDryRunChangesNothing(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "busy")

	r := w.ok("restart", "-n", "--ping")

	assertContains(t, r.stdout, "will be pinged")
	assertContains(t, r.stdout, "Dry run: nothing was changed.")
	if len(w.actions()) != 0 {
		t.Fatalf("dry run called %q", argsOf(w.actions()))
	}
}

func TestPauseThenResumeRoundTripsThroughTheParkFile(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "busy")
	beta := w.bg("beta", "idle")
	gamma := w.bg("gamma", "idle")
	w.pin(gamma)

	r := w.ok("pause")

	assertContains(t, r.stdout, "Parked 2 session(s) in "+w.parkPath())
	assertArgs(t, w.actions(), []string{"stop", alpha.ID}, []string{"stop", beta.ID})
	var parked struct {
		Version  int
		Sessions []struct {
			SessionID string `json:"session_id"`
			JobID     string `json:"job_id"`
			Cwd       string
			WasBusy   bool `json:"was_busy"`
		}
	}
	data, err := os.ReadFile(w.parkPath())
	if err != nil {
		t.Fatalf("no park file under XDG_STATE_HOME: %v", err)
	}
	if err := json.Unmarshal(data, &parked); err != nil || parked.Version != 1 || len(parked.Sessions) != 2 {
		t.Fatalf("park file %s, err %v", data, err)
	}
	if p := parked.Sessions[0]; p.SessionID != alpha.SessionID || p.JobID != alpha.ID || p.Cwd != alpha.Cwd || !p.WasBusy {
		t.Fatalf("alpha parked as %+v", p)
	}
	if p := parked.Sessions[1]; p.SessionID != beta.SessionID || p.WasBusy {
		t.Fatalf("beta parked as %+v", p)
	}
	st := w.state()
	if st["alpha"].PID != 0 || st["beta"].PID != 0 || st["gamma"].PID == 0 {
		t.Fatalf("after pause: %+v", st)
	}

	w.resetCalls()
	r = w.ok("resume", "--ping")

	assertContains(t, r.stdout, "resumed    alpha and pinged")
	assertContains(t, r.stdout, "resumed    beta")
	got := w.actions()
	assertArgs(t, got,
		[]string{"--bg", "--resume", alpha.SessionID, restartPing},
		[]string{"--bg", "--resume", beta.SessionID},
	)
	assertIn(t, got[0], alpha.Cwd)
	assertIn(t, got[1], beta.Cwd)
	if _, err := os.Stat(w.parkPath()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("park file left after everything resumed: %v", err)
	}
	if st := w.state(); st["alpha"].PID == 0 || st["beta"].PID == 0 {
		t.Fatalf("after resume: %+v", st)
	}
	if r := w.ok("resume"); !strings.Contains(r.stdout, "Nothing is parked.") {
		t.Fatalf("second resume printed %q", r.stdout)
	}
}

func TestStopTakesTheJobIDAndRefusesWhatIsNotRunning(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "idle")
	w.finished("old", "stopped", time.Hour)

	r := w.ok("stop", alpha.SessionID)

	assertContains(t, r.stdout, "stopped    alpha")
	assertArgs(t, w.actions(), []string{"stop", alpha.ID})
	if s := w.state()["alpha"]; s.PID != 0 || s.State != "stopped" {
		t.Fatalf("alpha after stop: %+v", s)
	}

	w.resetCalls()
	for _, target := range []string{"alpha", "old"} {
		r := w.run("stop", target)
		if r.code != 4 || !strings.Contains(r.stderr, "is not running") {
			t.Fatalf("stop %s: exit %d, stderr %q", target, r.code, r.stderr)
		}
	}
	if r := w.run("stop", "nobody"); r.code != 2 {
		t.Fatalf("stop nobody: exit %d", r.code)
	}
	if len(w.actions()) != 0 {
		t.Fatalf("refused stops called %q", argsOf(w.actions()))
	}
}

func TestGCRemovesOnlyOldUnpinnedFinishedSessions(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "idle")
	old := w.finished("old", "done", 30*24*time.Hour)
	failed := w.finished("crashed", "failed", 10*24*time.Hour)
	kept := w.finished("kept", "done", 30*24*time.Hour)
	w.finished("fresh", "stopped", time.Hour)
	// Started long ago, but its state.json changed today.
	touched := w.finished("touched", "done", 30*24*time.Hour)
	w.jobVersion(touched, "2.1.325")
	w.pin(kept)

	r := w.ok("gc", "--dry-run")
	assertContains(t, r.stdout, "Removing 2 session(s):")
	assertContains(t, r.stdout, "Dry run: nothing was changed.")
	if len(w.actions()) != 0 {
		t.Fatalf("gc --dry-run called %q", argsOf(w.actions()))
	}

	r = w.ok("gc")
	assertArgs(t, w.actions(), []string{"rm", old.ID}, []string{"rm", failed.ID})
	st := w.state()
	for _, name := range []string{"old", "crashed"} {
		if _, ok := st[name]; ok {
			t.Fatalf("%s still listed after gc", name)
		}
	}
	for _, name := range []string{"alpha", "kept", "fresh", "touched"} {
		if _, ok := st[name]; !ok {
			t.Fatalf("gc removed %s", name)
		}
	}
	if r := w.ok("gc"); !strings.Contains(r.stdout, "Nothing to clean up.") {
		t.Fatalf("second gc printed %q", r.stdout)
	}
}

func TestPassThroughReturnsClaudesExitCode(t *testing.T) {
	w := newWorld(t)
	w.agentsExit = 3

	r := w.run("--some-agents-flag", "two words")

	if r.code != 3 {
		t.Fatalf("exit %d, want 3; stderr %q", r.code, r.stderr)
	}
	if r.stdout != "agents --some-agents-flag two words\n" {
		t.Fatalf("stdout %q", r.stdout)
	}
	assertArgs(t, w.calls(), []string{"agents", "--some-agents-flag", "two words"})

	w.resetCalls()
	w.run()
	assertArgs(t, w.calls(), []string{"agents"})
}

func TestClaudesOwnFlagsDoNotPassThrough(t *testing.T) {
	w := newWorld(t)
	if r := w.ok("--version"); !strings.HasPrefix(r.stdout, "claudes version ") {
		t.Fatalf("--version printed %q", r.stdout)
	}
	if r := w.ok("--help"); !strings.Contains(r.stdout, "claudes lists, pins, restarts") {
		t.Fatalf("--help printed %q", r.stdout)
	}
	if len(w.calls()) != 0 {
		t.Fatalf("called claude: %q", argsOf(w.calls()))
	}
}

func TestClaudeRunsWithoutTheSessionsOwnVariables(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "busy")
	w.env = append(w.env, "CLAUDECODE=1", "CLAUDE_JOB_DIR="+filepath.Join(w.jobsDir(), "ff78d735"))

	w.ok("ls")
	w.ok("restart", "alpha", "--ping")
	w.run("--some-agents-flag")

	calls := w.calls()
	if len(calls) < 5 || calls[len(calls)-1].Args[0] != "agents" {
		t.Fatalf("calls %q", argsOf(calls))
	}
	for _, c := range calls {
		if c.ClaudeCode || c.ClaudeJobDir {
			t.Fatalf("%q ran with CLAUDECODE=%v CLAUDE_JOB_DIR=%v", c.Args, c.ClaudeCode, c.ClaudeJobDir)
		}
	}
}

func TestSkillPrintsAndInstalls(t *testing.T) {
	w := newWorld(t)
	want, err := os.ReadFile(filepath.Join("..", "..", "skills", "claudes", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	if r := w.ok("skill"); r.stdout != string(want) {
		t.Fatalf("claudes skill printed something other than skills/claudes/SKILL.md")
	}

	r := w.ok("skill", "--install")
	path := filepath.Join(w.claudeDir, "skills", "claudes", "SKILL.md")
	assertContains(t, r.stdout, "Installed the claudes skill at "+path)
	if got, _ := os.ReadFile(path); string(got) != string(want) {
		t.Fatalf("installed skill differs")
	}

	dir := filepath.Join(w.root, "agents", "skills")
	w.writeFile(filepath.Join(dir, "claudes", "SKILL.md"), "an older copy")
	w.ok("skill", "--dir", dir)
	if got, _ := os.ReadFile(filepath.Join(dir, "claudes", "SKILL.md")); string(got) != string(want) {
		t.Fatalf("--dir did not replace the older copy")
	}
	if len(w.calls()) != 0 {
		t.Fatalf("skill called claude: %q", argsOf(w.calls()))
	}
}

func TestCompleteStopOffersOnlyRunningSessions(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "idle")
	w.tty("console")
	w.finished("old", "stopped", time.Hour)

	r := w.ok("__complete", "stop", "")

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(r.stdout), "\n") {
		if strings.HasPrefix(line, ":") {
			continue
		}
		name, _, _ := strings.Cut(line, "\t")
		names = append(names, name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"alpha", "console"}) {
		t.Fatalf("completions %q from:\n%s", names, r.stdout)
	}
	if r.stderr != "" {
		t.Fatalf("completion wrote to stderr: %q", r.stderr)
	}
}

func TestAPromptThatLooksLikeAnOptionIsRefusedBeforeClaudeRuns(t *testing.T) {
	w := newWorld(t)
	w.bg("alpha", "busy")
	w.finished("old", "stopped", time.Hour)

	for _, args := range [][]string{
		{"restart", "alpha", "--ping-text=--dangerously-skip-permissions"},
		{"resume", "old", "--ping-text=-p"},
		{"ping", "old", "--", "--help"},
	} {
		r := w.run(args...)
		if r.code != 1 || !strings.Contains(r.stderr, "claude would read it as an option") {
			t.Fatalf("%q: exit %d, stderr %q", args, r.code, r.stderr)
		}
	}
	if len(w.calls()) != 0 {
		t.Fatalf("claude ran: %q", argsOf(w.calls()))
	}
}

func TestPingResumesAStoppedSessionWithThePrompt(t *testing.T) {
	w := newWorld(t)
	old := w.finished("old", "stopped", time.Hour)

	r := w.ok("ping", "old")

	assertContains(t, r.stdout, "pinged     old")
	got := w.actions()
	assertArgs(t, got, []string{"--bg", "--resume", old.SessionID, "Pick up where you left off."})
	assertIn(t, got[0], old.Cwd)
}

func TestLogsHandsOffByJobID(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "idle")
	w.tty("console")

	r := w.ok("logs", "alpha")

	if r.stdout != "logs of alpha\n" {
		t.Fatalf("stdout %q", r.stdout)
	}
	assertArgs(t, w.actions(), []string{"logs", alpha.ID})
	if r := w.run("attach", "console"); r.code != 4 || !strings.Contains(r.stderr, "is an interactive session") {
		t.Fatalf("attach console: exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestAFailureFromClaudeIsReportedWithWhatItPrinted(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "idle")
	w.failures = map[string]string{"respawn": "the job is wedged"}

	r := w.run("restart", "alpha")

	if r.code != 3 {
		t.Fatalf("exit %d", r.code)
	}
	assertContains(t, r.stderr, "1 session(s) need attention:")
	assertContains(t, r.stderr, "alpha: claude respawn "+alpha.ID+" failed: the job is wedged")
}

func TestAFailedListIsAnError(t *testing.T) {
	w := newWorld(t)
	w.failures = map[string]string{"agents": "not logged in"}

	r := w.run("ls")

	if r.code != 1 || r.stderr != "claudes: claude agents --json failed: not logged in\n" {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestARelativeConfigDirStopsEveryCommand(t *testing.T) {
	w := newWorld(t)
	w.env = append(w.env, "CLAUDE_CONFIG_DIR=relative")

	r := w.run("ls")

	if r.code != 1 || !strings.Contains(r.stderr, "CLAUDE_CONFIG_DIR must be an absolute path") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if len(w.calls()) != 0 {
		t.Fatalf("claude ran: %q", argsOf(w.calls()))
	}
}

func TestConfigSetChangesWhatPingSends(t *testing.T) {
	w := newWorld(t)
	old := w.finished("old", "stopped", time.Hour)

	w.ok("config", "set", "ping.text", "keep going")
	w.ok("ping", "old")

	if _, err := os.Stat(filepath.Join(w.configHome(), "claudes", "config.yaml")); err != nil {
		t.Fatalf("config.yaml is not under XDG_CONFIG_HOME: %v", err)
	}
	assertArgs(t, w.actions(), []string{"--bg", "--resume", old.SessionID, "keep going"})
}

func TestABrokenConfigStopsEveryCommandButConfig(t *testing.T) {
	w := newWorld(t)
	w.writeFile(filepath.Join(w.configHome(), "claudes", "config.yaml"), "wait:\n  timeout: soon\n")

	r := w.run("ls")

	if r.code != 1 || !strings.Contains(r.stderr, "claudes config edit") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if len(w.calls()) != 0 {
		t.Fatalf("claude ran: %q", argsOf(w.calls()))
	}
	w.ok("config", "set", "wait.timeout", "10m")
	w.ok("ls")
}

func TestDoctorCLIOnlyChecksTheRealCommandLine(t *testing.T) {
	w := newWorld(t)

	r := w.ok("doctor", "--cli-only", "--strict")

	assertContains(t, r.stdout, "every command and flag claudes calls is there")
	if got := argsOf(w.calls()); len(got) != 3 {
		t.Fatalf("calls %q, want --version, --help, and agents --help", got)
	}
}

func TestSessionResumeHandsOffInTheSessionsFolder(t *testing.T) {
	w := newWorld(t)
	dir := w.folder("plan")
	id := "5b8eecd0-75b4-442d-ac37-89f634538cb7"
	w.transcript(dir, id,
		map[string]string{"type": "user", "sessionId": id, "cwd": dir},
		map[string]string{"type": "custom-title", "customTitle": "the plan", "sessionId": id},
	)

	r := w.ok("session", "where", "--one", "the plan")
	if strings.TrimSpace(r.stdout) != dir {
		t.Fatalf("where printed %q", r.stdout)
	}
	r = w.ok("session", "resume", "the plan", "keep going")

	assertContains(t, r.stdout, "resumed "+id+" here")
	got := w.actions()
	assertArgs(t, got, []string{"--resume", id, "keep going"})
	assertIn(t, got[0], dir)
}

// folderName is how Claude Code names a working directory's project folder.
func folderName(dir string) string {
	return strings.Map(func(r rune) rune {
		if r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, dir)
}

// writeTeam writes the team config Claude Code keeps for a lead, with its
// teammates, and a task list.
func (w *world) writeTeam(lead fake.Session, teammates ...string) (team, tasks string) {
	w.t.Helper()
	name := "session-" + lead.SessionID[:8]
	members := []map[string]string{{"name": "team-lead", "agentType": "team-lead", "cwd": lead.Cwd}}
	for _, m := range teammates {
		members = append(members, map[string]string{"name": m, "agentType": "general-purpose", "cwd": lead.Cwd})
	}
	config, err := json.Marshal(map[string]any{"name": name, "leadSessionId": lead.SessionID, "members": members})
	if err != nil {
		w.t.Fatal(err)
	}
	team = filepath.Join(w.claudeDir, "teams", name)
	tasks = filepath.Join(w.claudeDir, "tasks", name)
	for path, data := range map[string]string{
		filepath.Join(team, "config.json"): string(config),
		filepath.Join(tasks, "1.json"):     `{"id":"1","subject":"write tests","status":"in_progress","owner":"tester"}`,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			w.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			w.t.Fatal(err)
		}
	}
	return team, tasks
}

func TestTeamLsAndGCTeams(t *testing.T) {
	w := newWorld(t)
	lead := w.bg("lead", "busy")
	gone := w.finished("gone", "done", 30*24*time.Hour)
	w.writeTeam(lead, "tester")
	oldTeam, oldTasks := w.writeTeam(gone)
	month := time.Now().Add(-30 * 24 * time.Hour)
	for _, p := range []string{filepath.Join(oldTeam, "config.json"), oldTeam, filepath.Join(oldTasks, "1.json"), oldTasks} {
		if err := os.Chtimes(p, month, month); err != nil {
			t.Fatal(err)
		}
	}

	r := w.ok("team", "ls")
	assertContains(t, r.stdout, "tester")
	assertContains(t, r.stdout, "0/1 done, working: tester")
	if strings.Contains(r.stdout, "gone") {
		t.Fatalf("team ls listed a team whose lead no longer runs:\n%s", r.stdout)
	}

	r = w.ok("gc", "--teams", "--dry-run")
	assertContains(t, r.stdout, "Dry run: nothing was changed.")
	if _, err := os.Stat(oldTeam); err != nil {
		t.Fatalf("a dry run removed %s: %v", oldTeam, err)
	}
	w.ok("gc", "--teams")
	for _, p := range []string{oldTeam, oldTasks} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("gc --teams kept %s", p)
		}
	}
	if r := w.ok("team", "ls"); !strings.Contains(r.stdout, "tester") {
		t.Fatalf("gc --teams touched the running lead's team:\n%s", r.stdout)
	}
}
