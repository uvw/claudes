package selection

import (
	"errors"
	"slices"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
)

func bg(job, name string) claude.Session {
	return claude.Session{JobID: job, SessionID: job + "-0000", Name: name, Kind: claude.KindBackground, State: "working", PID: 1}
}

func tty(id, name string) claude.Session {
	return claude.Session{SessionID: id + "-0000", Name: name, Kind: claude.KindInteractive, PID: 2}
}

func labels(sessions []claude.Session) []string {
	var out []string
	for _, s := range sessions {
		out = append(out, s.Label())
	}
	return out
}

func TestBulkSortsEachSessionByTheFirstReasonThatApplies(t *testing.T) {
	skipped := bg("aaaaaaaa", "skipped")
	pinnedTTY := tty("bbbbbbbb", "tty")
	noJob := bg("", "nojob")
	noJob.SessionID = "cccccccc-0000"
	pinned := bg("dddddddd", "pinned")
	lead := bg("eeeeeeee", "lead")
	pinnedLead := bg("ffffffff", "pinned-lead")
	chosen := bg("11111111", "chosen")
	running := []claude.Session{skipped, pinnedTTY, noJob, pinned, lead, pinnedLead, chosen}
	skip := map[string]bool{skipped.SessionID: true}
	pins := map[string]bool{"dddddddd": true, "ffffffff": true, "aaaaaaaa": true}
	teammates := func(id string) []string {
		if id == lead.SessionID || id == pinnedLead.SessionID || id == skipped.SessionID {
			return []string{"mate"}
		}
		return nil
	}

	sel := Bulk(running, skip, pins, false, teammates)
	for name, tc := range map[string]struct{ got, want []string }{
		"skipped":    {labels(sel.Skipped), []string{"skipped"}},
		"left alone": {labels(sel.LeftAlone), []string{"tty"}},
		"no job":     {labels(sel.NoJob), []string{"nojob"}},
		"pinned":     {labels(sel.Pinned), []string{"pinned", "pinned-lead"}},
		"leads":      {labels(sel.Leads), []string{"lead"}},
		"chosen":     {labels(sel.Chosen), []string{"chosen"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s: %q, want %q", name, tc.got, tc.want)
		}
	}
	if sel = Bulk(running, nil, nil, true, teammates); !slices.Equal(labels(sel.Chosen), []string{"tty", "pinned", "chosen"}) {
		t.Errorf("with interactive and no pins, chose %q", labels(sel.Chosen))
	}
}

func TestNamedTakesExactlyTheTargets(t *testing.T) {
	a, b, c := bg("aaaaaaaa", "api"), bg("bbbbbbbb", "docs"), tty("cccccccc", "shell")
	stopped := bg("dddddddd", "old")
	stopped.PID = 0
	sessions := []claude.Session{a, b, c, stopped}

	sel, err := Named([]string{"api", "docs"}, sessions, map[string]bool{b.SessionID: true}, false)
	if err != nil || !slices.Equal(labels(sel.Chosen), []string{"api"}) || !slices.Equal(labels(sel.Skipped), []string{"docs"}) {
		t.Fatalf("chose %q, skipped %q, err %v", labels(sel.Chosen), labels(sel.Skipped), err)
	}
	if _, err := Named([]string{"shell"}, sessions, nil, false); exit.Code(err) != exit.Refused {
		t.Errorf("an interactive session without --interactive: %v", err)
	}
	if sel, err := Named([]string{"shell"}, sessions, nil, true); err != nil || len(sel.Chosen) != 1 {
		t.Errorf("an interactive session with --interactive: %v", err)
	}
	if _, err := Named([]string{"old"}, sessions, nil, false); exit.Code(err) != exit.Refused {
		t.Errorf("a stopped session: %v", err)
	}
	if _, err := Named([]string{"nobody"}, sessions, nil, false); err == nil {
		t.Error("a target that matches nothing")
	}
}

func TestKeepOutdatedKeepsOlderBackgroundSessions(t *testing.T) {
	old, current, unknown, term := bg("aaaaaaaa", "old"), bg("bbbbbbbb", "current"), bg("cccccccc", "unknown"), tty("dddddddd", "tty")
	versions := map[string]string{old.JobID: "2.1.0", current.JobID: "2.2.0", term.SessionID: "1.0.0"}
	version := func(s claude.Session) string { return versions[s.JobID] }

	sel := Selection{Chosen: []claude.Session{old, current, unknown, term}}
	if KeepOutdated(&sel, "2.2.0", version) || !slices.Equal(labels(sel.Chosen), []string{"old"}) {
		t.Fatalf("kept %q", labels(sel.Chosen))
	}
	sel = Selection{Chosen: []claude.Session{unknown, term}}
	if !KeepOutdated(&sel, "2.2.0", version) || len(sel.Chosen) != 0 {
		t.Error("no readable version does not report unknown")
	}
	if sel = (Selection{Chosen: []claude.Session{term}}); KeepOutdated(&sel, "2.2.0", version) {
		t.Error("only interactive sessions report unknown")
	}
}

func TestSkipResolvesLikeTargets(t *testing.T) {
	sessions := []claude.Session{bg("aaaaaaaa", "api"), bg("bbbbbbbb", "api-2")}
	id := func(s claude.Session) string { return s.SessionID }
	if set, err := Skip(nil, sessions, id); err != nil || len(set) != 0 {
		t.Errorf("no skip: %v %v", set, err)
	}
	if set, err := Skip([]string{"api-2"}, sessions, id); err != nil || !set["bbbbbbbb-0000"] || len(set) != 1 {
		t.Errorf("skip api-2: %v %v", set, err)
	}
	if _, err := Skip([]string{"nobody"}, sessions, id); err == nil {
		t.Error("a skip that matches nothing")
	}
}

func TestFinishedAndRunning(t *testing.T) {
	done := bg("aaaaaaaa", "done")
	done.PID, done.State = 0, "done"
	idle := bg("bbbbbbbb", "idle")
	idle.PID, idle.State = 0, "idle"
	running, term := bg("cccccccc", "running"), tty("dddddddd", "tty")
	if !IsFinished(done) || IsFinished(idle) || IsFinished(running) {
		t.Error("IsFinished")
	}
	if err := RequireFinished([]claude.Session{done}); err != nil {
		t.Errorf("done: %v", err)
	}
	for _, s := range []claude.Session{idle, running, term} {
		if err := RequireFinished([]claude.Session{s}); exit.Code(err) != exit.Refused {
			t.Errorf("%s: %v", s.Label(), err)
		}
	}
	if err := RequireRunning([]claude.Session{running, done}); exit.Code(err) != exit.Refused || errors.Unwrap(err) == nil {
		t.Errorf("a finished session passes RequireRunning: %v", err)
	}
	if !RunningIn([]claude.Session{done, running}, func(s claude.Session) bool { return s.JobID == "cccccccc" }) ||
		RunningIn([]claude.Session{done, running}, func(s claude.Session) bool { return s.JobID == "aaaaaaaa" }) {
		t.Error("RunningIn")
	}
}

func TestTheHostGoesLastAndSplitsOff(t *testing.T) {
	a, h, b := bg("aaaaaaaa", "a"), bg("bbbbbbbb", "host"), bg("cccccccc", "c")
	for _, host := range []Host{{ID: h.SessionID}, {Job: "bbbbbbbb"}} {
		if got := labels(host.Last([]claude.Session{h, a, b})); !slices.Equal(got, []string{"a", "c", "host"}) {
			t.Errorf("%+v: order %q", host, got)
		}
		others, found := host.Split([]claude.Session{a, h, b})
		if found == nil || found.Label() != "host" || !slices.Equal(labels(others), []string{"a", "c"}) {
			t.Errorf("%+v: split %q and %v", host, labels(others), found)
		}
	}
	if others, found := (Host{}).Split([]claude.Session{a, b}); found != nil || len(others) != 2 {
		t.Error("no host split one off")
	}
	if (Host{}).IsID("") || !(Host{Job: "bbbbbbbb"}).IsID(h.SessionID) {
		t.Error("IsID")
	}
}

func TestTrueFirst(t *testing.T) {
	if TrueFirst(true, false) != -1 || TrueFirst(false, true) != 1 || TrueFirst(true, true) != 0 {
		t.Error("TrueFirst")
	}
}
