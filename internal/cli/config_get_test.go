package cli

import (
	"strings"
	"testing"
)

func TestConfigGetListsMCPProfiles(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "mcp:\n  profiles:\n    safe: [browser]\n")
	h.ok(t, "config", "get")
	assertContains(t, h.out.String(), `mcp.profiles   {"safe":["browser"]}`)
	h.out.Reset()
	h.ok(t, "config", "get", "mcp.profiles")
	if strings.TrimSpace(h.out.String()) != `{"safe":["browser"]}` {
		t.Fatalf("%s", h.out)
	}
	h.fails(t, "config", "set", "mcp.profiles", "safe")
	assertContains(t, h.err.String(), "claudes config edit")
}
