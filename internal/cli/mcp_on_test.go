package cli

import (
	"encoding/json"
	"testing"

	"github.com/uvw/claudes/internal/mcp"
)

func TestMCPOnTurnsOnByPatternAndLeavesWhatIsAlreadyOn(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	seen := h.fakeMod(t, s, modRuns)

	h.ok(t, "mcp", "on", "--all", "api")

	if len(*seen) != 1 || (*seen)[0].Args != "enable web-api" {
		t.Fatalf("requests %+v", *seen)
	}
	if got := h.disabled(t, s.Cwd); len(got) != 0 {
		t.Fatalf("disabled %q", got)
	}
	assertContains(t, h.out.String(), "already on")

	h.out.Reset()
	*seen = nil
	h.ok(t, "mcp", "off", "w*", "-o", "json", "api")
	var set mcp.Switch
	if err := json.Unmarshal(h.out.Bytes(), &set); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	if set.Via != mcp.ViaMod || set.Dir != s.Cwd || set.ID != s.JobID || len(set.Servers) != 1 ||
		set.Servers[0] != (mcp.Change{Name: "web-api", From: mcp.On, To: mcp.Off, Result: mcp.Done}) {
		t.Fatalf("%+v", set)
	}
}
