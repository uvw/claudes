package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestModStatusListsWhichRunningSessionsLoadedIt(t *testing.T) {
	h := newHarness(t)
	with, without, done := h.bg("with-mod", "idle"), h.tty("without-mod", "idle"), stopped(h.bg("finished", ""))
	h.agents.snapshots = [][]claude.Session{{with, without, done}}
	h.ok(t, "mod", "install", "--from", h.checkout(t))
	h.loaded(t, with)

	h.out.Reset()
	h.ok(t, "mod", "status")
	text := h.out.String()
	assertContains(t, text, "claudes-mod 0.1.0 is installed from")
	assertContains(t, text, "ID ")
	for line := range strings.Lines(text) {
		switch {
		case strings.Contains(line, "with-mod") && !strings.Contains(line, "without-mod"):
			assertContains(t, line, "0.1.0")
		case strings.Contains(line, "without-mod"):
			assertContains(t, line, "not loaded")
		case strings.Contains(line, "finished"):
			t.Fatalf("listed a session that is not running: %q", line)
		}
	}

	h.out.Reset()
	h.ok(t, "mod", "status", "-o", "json")
	var st modState
	if err := json.Unmarshal(h.out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.Enabled || len(st.Sessions) != 2 || st.Sessions[0].Version != "0.1.0" || st.Sessions[1].Version != "" {
		t.Fatalf("%+v", st)
	}
}
