package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/teams"
)

// Teammate names, task owners, and member directories come from config.json and
// task files, and reach the terminal without shell.Printable.
func TestTeamLsStripsControlCharacters(t *testing.T) {
	h := newHarness(t)
	h.app.colors = style.Palette{}
	lead := h.bg("lead", "busy")
	h.agents.snapshots = [][]claude.Session{{lead}}
	h.team(t, lead, "evil\x1b]0;pwned\x07\x1b[2J")
	tasks := filepath.Join(h.app.Teams.Tasks, teams.Name(lead.SessionID))
	writeFile(t, filepath.Join(tasks, "1.json"), `{"id":"1","subject":"x","status":"in_progress","owner":"own\u001b[31mer"}`)

	h.ok(t, "team", "ls")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Errorf("team ls printed an escape sequence: %q", h.out.String())
	}
	h.out.Reset()
	h.ok(t, "restart", "--ping", "lead", "--dry-run")
	if strings.ContainsRune(h.out.String(), 0x1b) {
		t.Errorf("restart --dry-run printed an escape sequence: %q", h.out.String())
	}
	h.out.Reset()
	h.err.Reset()
	h.ok(t, "stop", "lead")
	if strings.ContainsRune(h.err.String()+h.out.String(), 0x1b) {
		t.Fatalf("stop printed an escape sequence: %q", h.err.String())
	}
}
