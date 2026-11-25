package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestAnUnknownStatusStopsRestartUnlessForced(t *testing.T) {
	h := newHarness(t)
	odd := h.bg("odd", "thinking")
	h.agents.snapshots = [][]claude.Session{{odd}}

	h.fails(t, "restart", "odd")
	assertCalls(t, h)
	assertContains(t, h.err.String(), `odd has status "thinking"`)
	assertContains(t, h.err.String(), "--force")

	h.err.Reset()
	h.ok(t, "restart", "--force", "odd")
	assertCalls(t, h, "respawn "+odd.JobID)
	assertContains(t, h.err.String(), "going ahead with --force")
}

func TestEveryWriteCommandTakesForce(t *testing.T) {
	root := newHarness(t).app.rootCommand()
	for _, name := range []string{"restart", "pause", "resume", "ping", "stop", "gc", "pin", "unpin"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Lookup("force") == nil {
			t.Errorf("%s has no --force", name)
		}
	}
}

func TestPinRefusesAPinsFileItDoesNotRecognize(t *testing.T) {
	h := newHarness(t)
	alpha := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json"), `["session-7"]`)

	h.fails(t, "pin", "alpha")
	assertContains(t, h.err.String(), `pins.json holds "session-7", not a job id`)
	if got := readFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json")); got != `["session-7"]` {
		t.Fatalf("pins.json changed to %s", got)
	}

	h.ok(t, "pin", "--force", "alpha")
	assertContains(t, readFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json")), alpha.JobID)
}

func TestANewerClaudeCodeWarnsOnce(t *testing.T) {
	h := newHarness(t)
	alpha, beta := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha, beta}}
	h.agents.version = "2.9.0"

	h.ok(t, "restart")
	if n := strings.Count(h.err.String(), "Claude Code 2.9.0 is newer than claudes was tested with"); n != 1 {
		t.Fatalf("warned %d times:\n%s", n, h.err)
	}
	assertCalls(t, h, "respawn "+alpha.JobID, "respawn "+beta.JobID)
}

func TestTheVersionIsCached(t *testing.T) {
	h := newHarness(t)
	h.app.CacheDir = filepath.Join(h.root, "cache")
	h.app.Which = func(string) (string, error) { return "/bin/claude", nil }
	h.agents.version = "2.9.0"
	if _, err := h.app.installedVersion(t.Context()); err != nil {
		t.Fatal(err)
	}
	h.agents.version = "2.1.325"
	if v, _ := h.app.installedVersion(t.Context()); v != "2.9.0" {
		t.Fatalf("version %q, want the cached 2.9.0", v)
	}
}

func TestDoctorPassesOnAFamiliarClaudeCode(t *testing.T) {
	h := newHarness(t)
	alpha := h.bg("alpha", "busy")
	h.agents.snapshots = [][]claude.Session{{alpha, h.tty("term", "idle")}}
	h.pin(t, alpha)
	h.withConfig(t, "")

	h.ok(t, "doctor")
	for _, want := range []string{"claude", "command line", "agents --json", "pins.json", "pins lock", "transcripts", "config.yaml", "park file"} {
		assertContains(t, h.out.String(), want)
	}
	if strings.Contains(h.out.String(), "fail") {
		t.Fatalf("a check failed:\n%s", h.out)
	}
}

func TestDoctorWarnsWhenClaudeCodeMayWritePinsWithoutTheLock(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{}}
	h.withConfig(t, "")
	for _, tc := range []struct{ features, want string }{
		{`{"tengu_hover_rest":false}`, "ok      pins lock      Claude Code takes pins.json.lock"},
		{`{"tengu_hover_rest":true}`, "warn    pins lock      tengu_hover_rest is on"},
	} {
		writeFile(t, h.app.Accounts[0].MCP.ConfigFile, `{"cachedGrowthBookFeatures":`+tc.features+`}`)
		h.out.Reset()
		h.run("doctor")
		assertContains(t, h.out.String(), tc.want)
	}
	h.err.Reset()
	if code := h.run("doctor", "--strict"); code == 0 {
		t.Fatalf("doctor --strict passed with the flag on:\n%s", h.out)
	}
}

func TestDoctorFailsOnMissingCommandsAndUnknownValues(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("odd", "thinking")}}
	h.agents.help = map[string]string{"agents": "Usage: claude agents\n\nOptions:\n  --all   everything\n"}
	writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json"), `["x"]`)

	h.fails(t, "doctor", "-o", "json")
	var checks []check
	if err := json.Unmarshal(h.out.Bytes(), &checks); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	results := map[string]string{}
	for _, c := range checks {
		results[c.Name] = c.Result
	}
	for _, name := range []string{"command line", "agents --json", "pins.json"} {
		if results[name] != resultFail {
			t.Errorf("%s: %q, want fail", name, results[name])
		}
	}
	assertContains(t, h.err.String(), "3 check(s) failed")
}

func TestDoctorCLIOnlyAndStrict(t *testing.T) {
	h := newHarness(t)
	h.agents.version = "2.9.0"

	h.ok(t, "doctor", "--cli-only")
	if strings.Contains(h.out.String(), "agents --json") || h.agents.calls != nil {
		t.Fatalf("--cli-only listed sessions:\n%s\ncalls %v", h.out, h.agents.calls)
	}
	assertContains(t, h.out.String(), "warn")
	h.fails(t, "doctor", "--cli-only", "--strict")
}

func TestDoctorRunsWithABrokenConfig(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "nope: 1\n")
	h.fails(t, "doctor")
	assertContains(t, h.out.String(), "nope")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDoctorEnvPrintsVersionsFilesAndVariables(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	h.env("CLAUDES_CONFIG="+path, "VISUAL=")
	h.agents.version = "2.1.330"

	h.ok(t, "doctor", "env")
	for _, want := range []string{"claudes\n", "2.1.330", "tested", path + " (none", h.app.Accounts[0].Jobs.Dir, "CLAUDES_CONFIG          " + path, `VISUAL                  ""`} {
		assertContains(t, h.out.String(), want)
	}

	h.out.Reset()
	h.ok(t, "doctor", "env", "-o", "json")
	var report map[string]map[string]string
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	if report["claude"]["version"] != "2.1.330" || report["files"]["config"] == "" || report["environment"]["CLAUDES_CONFIG"] != path {
		t.Fatalf("report %v", report)
	}
}

func TestDoctorEnvRunsWithABrokenConfig(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "nope: 1\n")
	h.ok(t, "doctor", "env")
	assertContains(t, h.out.String(), "nope")
}

func TestCompletionTakesPwsh(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "completion", "pwsh")
	assertContains(t, h.out.String(), "Register-ArgumentCompleter")
	if len(h.execs) != 0 {
		t.Fatalf("passed through: %q", h.execs)
	}
}

func TestBulkCommandsRefuseAPinsFileTheyDoNotRecognize(t *testing.T) {
	for _, args := range [][]string{{"restart"}, {"pause"}, {"gc", "--older-than", "0d"}} {
		h := newHarness(t)
		alpha := h.bg("alpha", "idle")
		old := stopped(h.bg("old", ""))
		old.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
		h.agents.snapshots = [][]claude.Session{{alpha}}
		h.agents.finished = []claude.Session{old}
		writeFile(t, filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json"), `["`+alpha.SessionID+`"]`)

		h.fails(t, args...)
		assertCalls(t, h)
		assertContains(t, h.err.String(), "not a job id")
	}
}

func TestBulkCommandsRefuseAMissingJobsFolder(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
	if err := os.RemoveAll(h.app.Accounts[0].Jobs.Dir); err != nil {
		t.Fatal(err)
	}
	h.fails(t, "restart")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "CLAUDE_CONFIG_DIR may point to the wrong directory")
	h.err.Reset()
	h.ok(t, "restart", "--force")
}

// session mv removes a stopped job, and session resume --bg starts one, so
// both check what claude agents listed first.
func TestSessionMvAndResumeBgRefuseAListTheyDoNotRecognize(t *testing.T) {
	h := newHarness(t)
	s := h.stoppedJob(t, "plan", "--model", "m")
	h.agents.snapshots = [][]claude.Session{{h.bg("odd", "thinking")}}

	h.fails(t, "session", "mv", "plan", h.folder("to"))
	assertContains(t, h.err.String(), `odd has status "thinking"`)
	assertCalls(t, h)
	h.err.Reset()
	h.fails(t, "session", "resume", "--bg", "plan")
	assertContains(t, h.err.String(), `odd has status "thinking"`)
	assertCalls(t, h)

	h.ok(t, "session", "mv", "--force", "plan", h.folder("to"))
	assertCalls(t, h, "rm "+s.JobID)
	h.agents.calls = nil
	h.err.Reset()
	h.ok(t, "session", "resume", "--bg", "--force", "plan")
	assertContains(t, h.err.String(), "going ahead with --force")
}

func TestARunningSessionWithoutAStatusIsRefused(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "")}}
	h.fails(t, "restart", "--ping")
	assertCalls(t, h)
	assertContains(t, h.err.String(), "running but has no status")
}

func (h *harness) corruptPins(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(h.app.Accounts[0].Jobs.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.app.Accounts[0].Jobs.Dir, "pins.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBulkCommandsRefuseWhenPinsCannotBeRead(t *testing.T) {
	for _, args := range [][]string{{"restart"}, {"pause"}, {"gc"}} {
		t.Run(args[0], func(t *testing.T) {
			h := newHarness(t)
			old := stopped(h.bg("old", ""))
			old.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
			h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
			h.agents.finished = []claude.Session{old}
			h.corruptPins(t)

			h.fails(t, args...)
			assertCalls(t, h)
			assertContains(t, h.err.String(), "cannot read pins, so cannot tell which sessions to protect")
		})
	}
}
