package cli

import (
	"slices"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestMCPUseTurnsOnTheProfileAndOffTheRest(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	h.withConfig(t, "mcp:\n  profiles:\n    web: [web-api, issue-tracker]\n")
	seen := h.fakeMod(t, s, modRuns)

	h.ok(t, "mcp", "use", "web", "api")

	var args []string
	for _, r := range *seen {
		args = append(args, r.Args)
	}
	slices.Sort(args)
	if !slices.Equal(args, []string{"disable browser", "enable web-api"}) {
		t.Fatalf("requests %q", args)
	}
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"browser"}) {
		t.Fatalf("disabled %q", got)
	}
	assertContains(t, h.err.String(), `names "issue-tracker", which this directory does not have`)
}

func TestMCPUseRefusesAProfileThatIsNotThere(t *testing.T) {
	h := newHarness(t)
	h.mcpSession(t, "idle")
	h.withConfig(t, "mcp:\n  profiles:\n    web: [web-api]\n")
	h.fails(t, "mcp", "use", "nope", "api")
	assertContains(t, h.err.String(), "Profiles: web")
	h.fails(t, "mcp", "use", "web")
}

func TestMCPUseInADirectory(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	h.agents.snapshots = [][]claude.Session{{stopped(s)}}
	h.withConfig(t, "mcp:\n  profiles:\n    none: []\n")
	h.ok(t, "mcp", "use", "none", "--in", s.Cwd)
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"web-api", "browser"}) {
		t.Fatalf("disabled %q", got)
	}
}
