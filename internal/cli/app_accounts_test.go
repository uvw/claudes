package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/cswap"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/shell"
)

func assertOtherCalls(t *testing.T, f *fakeAgents, want ...string) {
	t.Helper()
	if diff := cmp.Diff(want, f.calls); diff != "" {
		t.Fatalf("the other account's calls (-want +got):\n%s", diff)
	}
}

func TestLsNamesTheAccountOnlyWhenThereAreSeveral(t *testing.T) {
	h := newHarness(t)
	alpha := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	h.ok(t, "ls")
	single := h.out.String()
	h.out.Reset()
	h.ok(t, "ls", "-o", "json")
	if strings.Contains(single, "ACCOUNT") || strings.Contains(h.out.String(), `"account"`) {
		t.Fatalf("one account shows an account:\n%s%s", single, h.out)
	}

	_, work := h.account("work")
	work.snapshots = [][]claude.Session{{h.bg("beta", "idle")}}
	h.out.Reset()
	h.ok(t, "ls")
	assertContains(t, h.out.String(), "ACCOUNT")
	for _, line := range strings.Split(h.out.String(), "\n") {
		switch {
		case strings.Contains(line, "alpha") && !strings.Contains(line, "~/.claude"),
			strings.Contains(line, "beta") && !strings.Contains(line, "work"):
			t.Fatalf("wrong account in %q", line)
		}
	}
	h.out.Reset()
	h.ok(t, "ls", "-o", "json")
	assertContains(t, h.out.String(), `"account": "work"`)
	h.out.Reset()
	h.ok(t, "ls", "--account", "work")
	if strings.Contains(h.out.String(), "alpha") || !strings.Contains(h.out.String(), "beta") {
		t.Fatalf("--account work listed:\n%s", h.out)
	}
}

func TestAnAccountNoneHasIsAnError(t *testing.T) {
	h := newHarness(t)
	h.account("work")
	h.fails(t, "ls", "--account", "play")
	assertContains(t, h.err.String(), `no account "play". Accounts: ~/.claude, work`)
}

// Each fake knows only its own account's jobs, so a call for beta made
// without the work account's directory fails with "No job matching".
func TestBulkRestartRespawnsEachSessionUnderItsOwnAccount(t *testing.T) {
	h := newHarness(t)
	_, work := h.account("work")
	alpha, beta := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	work.snapshots = [][]claude.Session{{beta}}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+alpha.JobID)
	assertOtherCalls(t, work, "respawn "+beta.JobID)
	assertContains(t, h.out.String(), "account work")
}

func TestAccountNarrowsABulkCommand(t *testing.T) {
	h := newHarness(t)
	_, work := h.account("work")
	alpha, beta := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	work.snapshots = [][]claude.Session{{beta}}

	h.ok(t, "restart", "--account", "work")

	assertCalls(t, h)
	assertOtherCalls(t, work, "respawn "+beta.JobID)
}

func TestBulkCommandsSkipASessionPinnedInItsOwnAccount(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	alpha, beta := h.bg("alpha", "idle"), h.bg("beta", "idle")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	work.snapshots = [][]claude.Session{{beta}}
	if _, err := acct.Jobs.SetPinned([]string{beta.JobID}, true); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+alpha.JobID)
	assertOtherCalls(t, work)
	assertContains(t, h.out.String(), "Pinned, not restarted: beta")
}

func TestPinWritesTheSessionsOwnAccountsPins(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	beta := h.bg("beta", "idle")
	work.snapshots = [][]claude.Session{{beta}}

	h.ok(t, "pin", "beta")

	if ids, _ := acct.Jobs.Pins(); !slices.Equal(ids, []string{beta.JobID}) {
		t.Fatalf("work's pins %v", ids)
	}
	if ids, _ := h.app.Accounts[0].Jobs.Pins(); len(ids) != 0 {
		t.Fatalf("own pins %v", ids)
	}
}

func TestTheCompatGateCoversEachAccountsJobsDirectory(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	work.snapshots = [][]claude.Session{{h.bg("beta", "idle")}}
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
	if err := os.RemoveAll(acct.Jobs.Dir); err != nil {
		t.Fatal(err)
	}
	h.exits(t, exitRefused, "restart")
	assertContains(t, h.err.String(), "the jobs directory "+acct.Jobs.Dir+" does not exist")
	assertCalls(t, h)

	writeFile(t, filepath.Join(acct.Jobs.Dir, "pins.json"), `["not a job id"]`)
	h.err.Reset()
	h.exits(t, exitRefused, "restart")
	assertContains(t, h.err.String(), "pins.json of account work holds something claudes does not recognize")
}

func TestStopPingAndGcReachTheSessionsAccount(t *testing.T) {
	h := newHarness(t)
	_, work := h.account("work")
	beta, old := h.bg("beta", "busy"), stopped(h.bg("old", "idle"))
	work.snapshots = [][]claude.Session{{beta}}
	work.finished = []claude.Session{old}

	h.ok(t, "ping", "beta", "-m", "go on")
	h.ok(t, "gc", "old")
	h.ok(t, "stop", "beta")

	assertCalls(t, h)
	assertOtherCalls(t, work,
		"stop "+beta.JobID, "resume "+beta.SessionID+" in beta prompt=go on",
		"rm "+old.JobID,
		"stop "+beta.JobID)
}

func TestLogsAndAttachHandOffUnderTheSessionsAccount(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	beta := h.bg("beta", "idle")
	work.snapshots = [][]claude.Session{{beta}}

	h.ok(t, "logs", "beta")
	h.ok(t, "attach", "beta")

	want := []claude.ConfigDir{acct.ConfigDir, acct.ConfigDir}
	if !slices.Equal(h.execConfigs, want) {
		t.Fatalf("exec configs %v, want %v", h.execConfigs, want)
	}
	if !slices.Equal(h.execs[1], []string{"claude", "attach", beta.JobID}) {
		t.Fatalf("exec %q", h.execs[1])
	}
}

func TestPauseRecordsTheAccountAndResumeBringsItBackThere(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	alpha, beta := h.bg("alpha", "idle"), h.bg("beta", "busy")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	work.snapshots = [][]claude.Session{{beta}}

	h.ok(t, "pause")

	entries, err := h.app.Park.Load()
	if err != nil || len(entries) != 2 {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	for _, e := range entries {
		want := park.Entry{Account: "~/.claude", AccountDir: h.root}
		if e.SessionID == beta.SessionID {
			want = park.Entry{Account: "work", AccountDir: acct.Dir}
		}
		if e.Account != want.Account || e.AccountDir != want.AccountDir {
			t.Errorf("%s: account %q in %q, want %q in %q", e.Name, e.Account, e.AccountDir, want.Account, want.AccountDir)
		}
	}

	h.agents.calls, work.calls = nil, nil
	h.ok(t, "resume", "--ping")

	assertCalls(t, h, "resume "+alpha.SessionID+" in alpha prompt=")
	assertOtherCalls(t, work, "resume "+beta.SessionID+" in beta prompt="+defaultPing)
	if entries, _ := h.app.Park.Load(); len(entries) != 0 {
		t.Fatalf("still parked: %+v", entries)
	}
}

// An entry from before claudes had accounts names none, and resumes under
// the account claudes runs as.
func TestAParkedEntryWithoutAnAccountResumesUnderClaudesOwn(t *testing.T) {
	h := newHarness(t)
	_, work := h.account("work")
	beta := stopped(h.bg("beta", "idle"))
	h.agents.finished = []claude.Session{beta}
	if err := h.app.Park.Save([]park.Entry{park.FromSession(beta, false, "")}); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "resume")

	assertCalls(t, h, "resume "+beta.SessionID+" in beta prompt=")
	assertOtherCalls(t, work)
}

func TestAnInteractiveSessionOfAnotherAccountPrintsItsAccountsCommand(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	gamma := h.tty("gamma", "idle")
	work.snapshots = [][]claude.Session{{gamma}}

	h.ok(t, "stop", "gamma")

	if !slices.Equal(h.terminated, []int{gamma.PID}) || !h.terminatedSince[0].Equal(gamma.Started()) {
		t.Fatalf("terminated %v since %v", h.terminated, h.terminatedSince)
	}
	assertContains(t, h.out.String(), shell.ResumeCommandIn(acct.Dir, gamma.SessionID, gamma.Cwd, true))
}

func TestTheHostGoesLastAcrossAccounts(t *testing.T) {
	h := newHarness(t)
	_, work := h.account("work")
	alpha, beta := h.bg("alpha", "busy"), h.bg("beta", "busy")
	h.agents.snapshots = [][]claude.Session{{alpha}}
	work.snapshots = [][]claude.Session{{beta}}
	h.host(alpha)
	order := []string{}
	h.agents.beforeRespawn = func(string) { order = append(order, "alpha") }
	work.beforeStop = func(string) { order = append(order, "beta") }

	h.ok(t, "restart", "--ping")

	if !slices.Equal(order, []string{"beta", "alpha"}) {
		t.Fatalf("order %v, want beta, then the host", order)
	}
	assertCalls(t, h, "respawn "+alpha.JobID)
}

func TestSessionResumeRunsUnderClaudesOwnAccountOrTheNamedOne(t *testing.T) {
	h := newHarness(t)
	acct, _ := h.account("work")
	// Windows makes a file link to a target that does not exist yet.
	if err := os.MkdirAll(h.app.Transcripts.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(h.app.Transcripts.Dir, filepath.Join(acct.Dir, "projects")); err != nil {
		t.Skip("cannot make a link:", err)
	}
	dir := h.folder("gamma")
	id := h.transcript(t, "gamma", dir, time.Hour)

	h.ok(t, "session", "resume", "gamma")
	h.ok(t, "session", "resume", "gamma", "--account", "work")

	want := []claude.ConfigDir{{}, acct.ConfigDir}
	if !slices.Equal(h.execConfigs, want) {
		t.Fatalf("exec configs %v, want %v", h.execConfigs, want)
	}
	if !slices.Equal(h.execs[1], []string{"claude", "--resume", id}) {
		t.Fatalf("exec %q", h.execs[1])
	}
}

func TestSessionLsListsASharedProjectsDirectoryOnce(t *testing.T) {
	h := newHarness(t)
	acct, _ := h.account("work")
	// Windows makes a file link to a target that does not exist yet.
	if err := os.MkdirAll(h.app.Transcripts.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(h.app.Transcripts.Dir, filepath.Join(acct.Dir, "projects")); err != nil {
		t.Skip("cannot make a link:", err)
	}
	h.transcript(t, "gamma", h.folder("gamma"), time.Hour)

	h.ok(t, "session", "ls")

	if n := strings.Count(h.out.String(), "\n") - 1; n != 1 {
		t.Fatalf("gamma listed %d times:\n%s", n, h.out)
	}
}

func TestSessionGcRemovesFileHistoryInEveryAccount(t *testing.T) {
	h := newHarness(t)
	acct, _ := h.account("work")
	id := h.transcript(t, "gamma", h.folder("gamma"), 60*24*time.Hour)
	for _, home := range []string{h.root, acct.Dir} {
		writeFile(t, filepath.Join(home, "file-history", id, "f@v1"), "x")
	}

	h.ok(t, "session", "gc", "gamma")

	for _, home := range []string{h.root, acct.Dir} {
		if _, err := os.Stat(filepath.Join(home, "file-history", id)); !os.IsNotExist(err) {
			t.Errorf("file history left in %s: %v", home, err)
		}
	}
}

func TestTeamLsAndMCPLsCoverEachAccount(t *testing.T) {
	h := newHarness(t)
	acct, work := h.account("work")
	beta := h.bg("beta", "busy")
	work.snapshots = [][]claude.Session{{beta}}
	h.team(t, beta, "tester")
	if err := os.Rename(h.app.Accounts[0].Teams.Teams, acct.Teams.Teams); err != nil {
		t.Fatal(err)
	}
	h.ok(t, "team", "ls")
	assertContains(t, h.out.String(), "ACCOUNT")
	assertContains(t, h.out.String(), "work")

	writeFile(t, acct.MCP.ConfigFile, `{"mcpServers": {"docs": {"command": "docs"}}}`)
	h.out.Reset()
	h.ok(t, "mcp", "ls", "--in", h.folder("beta"))
	assertContains(t, h.out.String(), "under account work:")
	assertContains(t, h.out.String(), "No MCP servers in")
	h.out.Reset()
	h.ok(t, "mcp", "ls", "--in", h.folder("beta"), "-o", "json", "--account", "work")
	if !strings.HasPrefix(strings.TrimSpace(h.out.String()), "[") || !strings.Contains(h.out.String(), `"account": "work"`) {
		t.Fatalf("json:\n%s", h.out)
	}
}

func TestDoctorListsAccountsAndChecksCswap(t *testing.T) {
	h := newHarness(t)
	acct, _ := h.account("work")
	stale := filepath.Join(h.root, "backup", "sessions", "3-old_example.com")
	staleDir := claude.ConfigDir{Path: stale}
	h.agents.others[staleDir] = &fakeAgents{snapshots: [][]claude.Session{{h.bg("orphan", "idle")}}}
	h.app.Found.Cswap = true
	h.app.Found.Sequence = cswap.Sequence{Active: "1", Accounts: []cswap.Account{{Number: "1", Email: "you@example.com"}}}
	h.app.Found.Skipped = []cswap.Profile{{Dir: stale, Skipped: "account 3 is new@example.com now", Jobs: true}}
	h.app.Which = func(name string) (string, error) { return "/bin/" + name, nil }
	var ran string
	h.app.CswapList = func(_ context.Context, binary string) ([]byte, error) {
		ran = binary
		return []byte(`{"schemaVersion":1,"activeAccountNumber":1,"accounts":[{"number":1,"email":"you@example.com"},{"number":2,"email":"two@example.com"}]}`), nil
	}

	h.run("doctor")

	out := h.out.String()
	if ran != "/bin/cswap" {
		t.Errorf("ran %q", ran)
	}
	for _, want := range []string{
		"~/.claude: " + h.root + ", from ~/.claude",
		"work: " + acct.Dir + ", from config.yaml",
		"cswap lists account 2 (two@example.com), which sequence.json lacks",
		"runs 1 background session(s)",
		"add it to config.yaml under accounts.dirs, such as: old: " + stale,
		"agents --json (work)",
		"pins.json (~/.claude)",
	} {
		assertContains(t, out, want)
	}
}

func TestDoctorRunsNoCswapWithoutIt(t *testing.T) {
	h := newHarness(t)
	h.app.Which = func(name string) (string, error) { return "", os.ErrNotExist }
	h.app.CswapList = func(context.Context, string) ([]byte, error) {
		t.Fatal("ran cswap")
		return nil, nil
	}
	h.run("doctor")
	if strings.Contains(h.out.String(), "cswap") || strings.Contains(h.out.String(), "accounts") {
		t.Fatalf("doctor:\n%s", h.out)
	}
}
