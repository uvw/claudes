package compat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/jobs"
)

// TestLiveClaude reads the real `claude agents --json` and each background
// job's state.json with the code claudes uses. CLAUDES_LIVE_CLAUDE=list allows
// an empty list, and job needs a background job. A job that just started has
// no status and leaves keys out of its state.json, so the checks retry for up
// to a minute. It runs only in CI, as compat-live.sh does, so a local go test
// never reaches the Claude Code on the machine it runs on.
func TestLiveClaude(t *testing.T) {
	mode := os.Getenv("CLAUDES_LIVE_CLAUDE")
	switch {
	case mode == "":
		t.Skip("set CLAUDES_LIVE_CLAUDE to list or job to check the real claude")
	case os.Getenv("CI") != "true":
		t.Skip("CLAUDES_LIVE_CLAUDE runs only in CI, where compat-live.sh starts its job")
	}
	switch mode {
	case "list", "job":
	default:
		t.Fatalf("CLAUDES_LIVE_CLAUDE is %q, not list or job", mode)
	}
	store, err := jobs.DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	client := claude.NewClient()
	if v, err := client.Version(t.Context()); err == nil {
		t.Logf("Claude Code %s, jobs in %s", v, store.Dir)
	}

	var sessions []claude.Session
	var problems []string
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(time.Second) {
		sessions, problems = checkLive(t.Context(), t, client, store, mode == "job")
		if len(problems) == 0 || mode == "list" || time.Now().After(deadline) {
			break
		}
	}
	for _, s := range sessions {
		t.Logf("listed %s: kind %q, state %q, status %q", s.ID(), s.Kind, s.State, s.Status)
	}
	if len(problems) != 0 {
		t.Fatal(strings.Join(problems, "\n"))
	}
}

// checkLive lists the real sessions and checks them and each background
// job's state.json. With wantJob, a list with no background job fails.
func checkLive(ctx context.Context, t *testing.T, client *claude.Client, store jobs.Store, wantJob bool) ([]claude.Session, []string) {
	t.Helper()
	sessions, err := client.Sessions(ctx, claude.ConfigDir{}, true)
	if err != nil {
		t.Fatal(err)
	}
	problems := CheckSessions(sessions)
	found := 0
	for _, s := range sessions {
		if s.JobID == "" {
			continue
		}
		found++
		if p := checkJob(store, s); len(p) != 0 {
			state, _ := os.ReadFile(filepath.Join(store.Dir, s.JobID, "state.json"))
			problems = append(problems, p...)
			problems = append(problems, "state.json:\n"+string(state))
		}
	}
	if wantJob && found == 0 {
		problems = append(problems, "claude agents --json lists no background job")
	}
	return sessions, problems
}
