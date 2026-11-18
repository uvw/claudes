package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/jobs"
)

// The samples in testdata come from a real Claude Code 2.1.327, with made-up
// ids, names, and paths: `claude agents --json --all` listing one background
// job, and that job's state.json. The job was never logged in, so it never
// took a turn.
const (
	agentsSample = "agents-2.1.327.json"
	stateSample  = "state-2.1.327.json"
)

// checkJob lists what claudes cannot read in a background session's
// state.json, through the jobs.Store readers the commands use. It leaves out
// cliVersion, which Claude Code writes only once a job takes a turn.
func checkJob(store jobs.Store, s claude.Session) []string {
	who := s.Label()
	if store.LastActive(s.JobID).IsZero() {
		return []string{fmt.Sprintf("%s has no state.json in %s", who, store.Dir)}
	}
	var problems []string
	if !store.Has(s.JobID, s.SessionID) {
		problems = append(problems, fmt.Sprintf("the state.json of %s does not give its session id as sessionId", who))
	}
	if dir := store.Folder(s.JobID, s.SessionID); dir != s.Cwd {
		problems = append(problems, fmt.Sprintf("the state.json of %s gives cwd %q, not %q", who, dir, s.Cwd))
	}
	if _, ok := store.Options(s.JobID, s.SessionID); !ok {
		problems = append(problems, fmt.Sprintf("the state.json of %s has no respawnFlags list", who))
	}
	if a := store.Live(s.JobID); a.Shells == nil || a.PRs == nil {
		problems = append(problems, fmt.Sprintf("the state.json of %s has no readable fan, inFlight, or children", who))
	}
	return problems
}

func readSample(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parseSample(t *testing.T, data []byte) []claude.Session {
	t.Helper()
	var sessions []claude.Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		t.Fatalf("not a list of sessions: %v", err)
	}
	return sessions
}

// sampleStore is a jobs directory that holds state for s's job.
func sampleStore(t *testing.T, s claude.Session, state []byte) jobs.Store {
	t.Helper()
	store := jobs.Store{Dir: t.TempDir()}
	dir := filepath.Join(store.Dir, s.JobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), state, 0o644); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestTheRealAgentsListParses(t *testing.T) {
	sessions := parseSample(t, readSample(t, agentsSample))
	if p := CheckSessions(sessions); len(p) != 0 {
		t.Fatalf("problems %q", p)
	}
	if len(sessions) != 1 {
		t.Fatalf("%d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if s.JobID != "5e1f0c3a" || s.Kind != claude.KindBackground || s.Cwd != "/home/user/project" ||
		s.Name != "say hi" || !s.Running() || s.Started().IsZero() || s.MidTurn() {
		t.Fatalf("session %+v", s)
	}

	if empty := parseSample(t, []byte("[]\n")); len(empty) != 0 || len(CheckSessions(empty)) != 0 {
		t.Fatalf("an empty list gave %+v", empty)
	}
}

func TestTheRealStateFileParses(t *testing.T) {
	s := parseSample(t, readSample(t, agentsSample))[0]
	state := readSample(t, stateSample)
	store := sampleStore(t, s, state)
	if p := checkJob(store, s); len(p) != 0 {
		t.Fatalf("problems %q", p)
	}
	if options, _ := store.Options(s.JobID, s.SessionID); len(options) != 0 {
		t.Fatalf("options %q", options)
	}
	if a := store.Live(s.JobID); *a.Subagents != 0 || *a.Teammates != 0 || *a.Shells != 0 || *a.PRs != 0 || a.Doing != nil || a.Tokens != nil {
		t.Fatalf("activity %+v", a)
	}
}

func TestCheckJobFindsWhatChanged(t *testing.T) {
	s := parseSample(t, readSample(t, agentsSample))[0]
	state := string(readSample(t, stateSample))
	for _, tc := range []struct{ name, from, to, want string }{
		{"a renamed sessionId", `"sessionId"`, `"session"`, "sessionId"},
		{"a renamed cwd", `"cwd"`, `"dir"`, `cwd ""`},
		{"no respawnFlags", `"respawnFlags"`, `"flags"`, "respawnFlags"},
		{"a renamed inFlight", `"inFlight"`, `"running"`, "inFlight"},
		{"children as an object", `"children": null`, `"children": {}`, "children"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(state, tc.from, tc.to, 1)
			if changed == state {
				t.Fatalf("the sample has no %s", tc.from)
			}
			p := checkJob(sampleStore(t, s, []byte(changed)), s)
			if len(p) == 0 || !strings.Contains(strings.Join(p, "\n"), tc.want) {
				t.Fatalf("problems %q, want one about %s", p, tc.want)
			}
		})
	}
	if p := checkJob(jobs.Store{Dir: t.TempDir()}, s); len(p) != 1 || !strings.Contains(p[0], "no state.json") {
		t.Fatalf("a missing state.json: %q", p)
	}
}
