package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestSkipThatMatchesNothingStopsTheCommand(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("db-migration", "idle"), h.bg("other", "idle")}}

	h.fails(t, "restart", "--skip", "db-migrate")

	assertCalls(t, h)
	assertContains(t, h.err.String(), `--skip: no session matches "db-migrate"`)
}

func TestSkipMatchesLikeATargetAndIsListed(t *testing.T) {
	h := newHarness(t)
	db, other := h.bg("db-migration", "idle"), h.bg("other", "idle")
	h.agents.snapshots = [][]claude.Session{{db, other}}

	h.ok(t, "restart", "--skip", "DB")

	assertCalls(t, h, "respawn "+other.JobID)
	assertContains(t, h.out.String(), "Skipped, not restarted: db-migration")
}

func TestResumeSkipsAndTargetsAParkedSessionByJobID(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("alpha", "idle"), h.bg("bravo", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b}}
	h.ok(t, "pause")
	h.world([]claude.Session{})

	h.ok(t, "resume", "--skip", b.JobID)
	assertCalls(t, h, "resume "+a.SessionID+" in alpha prompt=")

	h.world([]claude.Session{})
	h.ok(t, "resume", b.JobID)
	assertCalls(t, h, "resume "+b.SessionID+" in bravo prompt=")
}

func TestBulkCommandsLeaveABackgroundSessionWithoutAJobIDAlone(t *testing.T) {
	h := newHarness(t)
	a, odd := h.bg("alpha", "idle"), h.bg("odd", "idle")
	odd.JobID = ""
	h.agents.snapshots = [][]claude.Session{{a, odd}}

	h.ok(t, "restart")

	assertCalls(t, h, "respawn "+a.JobID)
	assertContains(t, h.out.String(), "gave no job id: odd")
}

func TestGCLeavesParkedAndRecentlyActiveSessionsAlone(t *testing.T) {
	h := newHarness(t)
	old, parkedOld, recent := stopped(h.bg("old", "")), stopped(h.bg("parked-old", "")), stopped(h.bg("recent", ""))
	for _, s := range []*claude.Session{&old, &parkedOld, &recent} {
		s.StartedAt = h.now.Add(-30 * 24 * time.Hour).UnixMilli()
	}
	writeState(t, h, recent.JobID, "2.1.325")
	h.agents.finished = []claude.Session{old, parkedOld, recent}
	if err := h.app.parkOne(parkedOld, false); err != nil {
		t.Fatal(err)
	}

	h.ok(t, "gc")

	assertCalls(t, h, "rm "+old.JobID)
}

func TestOutdatedWarnsWhenNoVersionCanBeRead(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	assertContains(t, h.err.String(), "cannot read the Claude Code version of any session")
}

func TestOutdatedWithEverySessionPinnedDoesNotWarn(t *testing.T) {
	h := newHarness(t)
	p := h.bg("pinned", "idle")
	h.agents.snapshots = [][]claude.Session{{p}}
	h.pin(t, p)

	h.ok(t, "restart", "--outdated")

	assertCalls(t, h)
	if strings.Contains(h.err.String(), "cannot read the Claude Code version") {
		t.Fatalf("warned about versions with no session to check:\n%s", h.err)
	}
}

func TestHomeShort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sep := string(filepath.Separator)
	for path, want := range map[string]string{
		home:                                  "~",
		filepath.Join(home, "work", "repo"):   "~" + sep + filepath.Join("work", "repo"),
		home + "-other":                       home + "-other",
		filepath.Join(os.TempDir(), "x", "y"): filepath.Join(os.TempDir(), "x", "y"),
	} {
		if got := homeShort(path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}

func TestSkillPrintsAndInstalls(t *testing.T) {
	h := newHarness(t, nil)
	if code := h.run("skill"); code != 0 || !strings.HasPrefix(h.out.String(), "---\nname: claudes\n") {
		t.Fatalf("exit %d, output:\n%s", code, h.out)
	}
	printed := h.out.String()

	config := filepath.Join(h.root, "config")
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	h.ok(t, "skill", "--install")
	data, err := os.ReadFile(filepath.Join(config, "skills", "claudes", "SKILL.md"))
	if err != nil || string(data) != printed {
		t.Fatalf("installed %q, err %v", data, err)
	}

	other := filepath.Join(h.root, "agents-skills")
	h.ok(t, "skill", "--dir", other)
	if _, err := os.Stat(filepath.Join(other, "claudes", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestANameOfAStoppedSessionNeverPicksARunningOne(t *testing.T) {
	h := newHarness(t)
	gw := h.bg("api-gw", "idle")
	h.agents.snapshots = [][]claude.Session{{gw}}
	h.agents.finished = []claude.Session{stopped(h.bg("api", ""))}

	for _, cmd := range []string{"restart", "stop", "pause"} {
		h.err.Reset()
		h.fails(t, cmd, "api")
		assertContains(t, h.err.String(), "api is not running")
	}
	assertCalls(t, h)
}

func TestForgetDryRunDropsNothing(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}
	h.ok(t, "pause")
	h.world([]claude.Session{})

	h.ok(t, "resume", "--forget", "--dry-run", "alpha")

	assertContains(t, h.out.String(), "Would drop from the park list: alpha")
	if entries, _ := h.app.Park.Load(); len(entries) != 1 {
		t.Fatalf("entries %+v", entries)
	}
}

func TestStopParksAnInteractiveSessionAndPrintsHowToResumeIt(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	h.agents.snapshots = [][]claude.Session{{term}}

	h.ok(t, "stop", "terminal")

	assertContains(t, h.out.String(), "claude --resume "+term.SessionID)
	if entries, _ := h.app.Park.Load(); len(entries) != 1 || entries[0].SessionID != term.SessionID {
		t.Fatalf("parked %+v", entries)
	}
}

func TestBuildVersionPrefersTheReleaseVersion(t *testing.T) {
	defer func(v, c string) { Version, Commit = v, c }(Version, Commit)
	Version, Commit = "0.3.0", "abc1234"
	if got := buildVersion(); got != "0.3.0 (abc1234)" {
		t.Fatalf("got %q", got)
	}
	Commit = ""
	if got := buildVersion(); got != "0.3.0" {
		t.Fatalf("got %q", got)
	}
}

func TestAgeIsShortAndCoarse(t *testing.T) {
	for ago, want := range map[time.Duration]string{
		10 * time.Second: "now",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	} {
		if got := age(start, start.Add(-ago)); got != want {
			t.Errorf("%s ago: got %q, want %q", ago, got, want)
		}
	}
	if got := age(start, time.Time{}); got != "-" {
		t.Errorf("no start time: got %q", got)
	}
}

func TestOlderThanShowsItsDefaultInDays(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "gc", "--help")
	assertContains(t, h.out.String(), "--older-than duration")
	assertContains(t, h.out.String(), "(default 7d)")
	if s := (&days{d: 90 * time.Minute}).String(); s != "1h30m0s" {
		t.Fatalf("90m: %q", s)
	}
}
