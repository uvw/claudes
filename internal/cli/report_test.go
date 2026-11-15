package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/claude"
)

func readReport(t *testing.T, h *harness) report {
	t.Helper()
	var r report
	if err := json.Unmarshal(h.out.Bytes(), &r); err != nil {
		t.Fatalf("stdout is not one JSON report: %v\n%s", err, h.out)
	}
	return r
}

func results(entries []reportEntry) []string {
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name+" "+e.Result)
	}
	return out
}

func TestRestartPrintsOneJSONReport(t *testing.T) {
	h := newHarness(t)
	good, bad, pinned := h.bg("good", "busy"), h.bg("bad", "idle"), h.bg("kept", "idle")
	h.agents.snapshots = [][]claude.Session{{good, bad, pinned}}
	h.pin(t, pinned)
	h.agents.failures = map[string]string{"respawn " + bad.JobID: "boom"}

	h.exits(t, exitSomeFailed, "restart", "--ping", "-o", "json")
	r := readReport(t, h)
	if r.Command != "restart" || r.DryRun || len(r.Plan) != 2 {
		t.Fatalf("report %+v", r)
	}
	if diff := cmp.Diff([]string{"good restarted", "bad failed"}, results(r.Results)); diff != "" {
		t.Fatalf("results (-want +got):\n%s", diff)
	}
	if r.Results[0].ID != good.JobID || r.Results[0].Note != "and pinged" {
		t.Fatalf("first result %+v", r.Results[0])
	}
	if len(r.Failures) != 1 || r.Failures[0].Name != "bad" || !strings.Contains(r.Failures[0].Error, "boom") {
		t.Fatalf("failures %+v", r.Failures)
	}
	if diff := cmp.Diff([]string{"kept pinned"}, results(r.LeftOut)); diff != "" {
		t.Fatalf("left out (-want +got):\n%s", diff)
	}
	assertContains(t, h.err.String(), "need attention")
}

func TestADryRunReportHasThePlanAndEveryKey(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "pause", "--dry-run", "-o", "yaml")
	var r map[string]any
	if err := yaml.Unmarshal(h.out.Bytes(), &r); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	for _, key := range []string{"command", "dryRun", "plan", "results", "leftOut", "manual", "failures", "notReached", "warnings", "error"} {
		if _, ok := r[key]; !ok {
			t.Errorf("no %q in\n%s", key, h.out)
		}
	}
	if r["dryRun"] != true || len(r["plan"].([]any)) != 1 || len(r["results"].([]any)) != 0 {
		t.Fatalf("report %v", r)
	}
	if len(h.agents.calls) != 0 {
		t.Fatalf("a dry run acted: %q", h.agents.calls)
	}
}

func TestTheReportIsOutBeforeTheHostStops(t *testing.T) {
	h := newHarness(t)
	other, host := h.bg("other", "idle"), h.bg("host", "idle")
	h.agents.snapshots = [][]claude.Session{{other, host}}
	h.host(host)

	h.ok(t, "stop", "other", "host", "-o", "json")
	r := readReport(t, h)
	if diff := cmp.Diff([]string{"other stopped", "host stopping"}, results(r.Results)); diff != "" {
		t.Fatalf("results (-want +got):\n%s", diff)
	}
}

func TestPinReportsEachSession(t *testing.T) {
	h := newHarness(t)
	a := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "pin", "api", "-o", "json")
	if diff := cmp.Diff([]string{"api pinned"}, results(readReport(t, h).Results)); diff != "" {
		t.Fatalf("results (-want +got):\n%s", diff)
	}
}

func TestResultsCarryTheFolder(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "stop", "api", "-o", "json")
	if r := readReport(t, h); len(r.Results) != 1 || r.Results[0].Cwd != s.Cwd {
		t.Fatalf("results %+v, want cwd %s", r.Results, s.Cwd)
	}
}

func TestOutdatedWithNothingOutdatedSaysSo(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "restart", "--outdated", "--dry-run")
	assertContains(t, h.out.String(), "No running session is on an older Claude Code")
}

func TestTheReportListsLeftAloneTerminalsWarningsAndTheError(t *testing.T) {
	h := newHarness(t)
	api, term := h.bg("api", "idle"), h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{api, term}}

	h.ok(t, "restart", "-o", "json")
	r := readReport(t, h)
	if len(r.Manual) != 0 || len(r.LeftOut) != 1 || r.LeftOut[0].Name != "terminal" ||
		r.LeftOut[0].Note != "interactive" || !strings.Contains(r.LeftOut[0].Command, term.SessionID) {
		t.Fatalf("manual %+v, left out %+v", r.Manual, r.LeftOut)
	}
	if r.Error != "" || r.Warnings == nil {
		t.Fatalf("error %q, warnings %v", r.Error, r.Warnings)
	}

	h.out.Reset()
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "tester")
	h.ok(t, "stop", "lead", "-o", "json")
	r = readReport(t, h)
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "leads 1 teammate(s): tester") {
		t.Fatalf("warnings %q", r.Warnings)
	}

	h.out.Reset()
	h.exits(t, exitNoMatch, "stop", "nobody", "-o", "json")
	if r = readReport(t, h); !strings.Contains(r.Error, "nobody") {
		t.Fatalf("error %q", r.Error)
	}
}
