package cli

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestMCPProfilesListsThemAndWarnsAboutUnknownNames(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("api")
	h.mcpServers(t, dir, map[string]any{"enabledMcpjsonServers": []string{"other-repo-tool"}})
	h.withConfig(t, "mcp:\n  profiles:\n    safe: [browser, other-repo-tool, typo-server]\n    none: []\n")

	h.ok(t, "mcp", "profiles")
	text := h.out.String()
	if !regexp.MustCompile(`(?m)^NAME\s+SERVERS$`).MatchString(text) ||
		!regexp.MustCompile(`(?m)^none\s+-$`).MatchString(text) ||
		!regexp.MustCompile(`(?m)^safe\s+browser, other-repo-tool, typo-server$`).MatchString(text) {
		t.Fatalf("table:\n%s", text)
	}
	assertContains(t, h.err.String(), `profile safe names "typo-server", which matches no MCP server`)
	if strings.Contains(h.err.String(), "other-repo-tool") || strings.Contains(h.err.String(), `"browser"`) {
		t.Fatalf("warned about a known server:\n%s", h.err)
	}

	h.out.Reset()
	h.ok(t, "mcp", "profiles", "-o", "json")
	var got []mcpProfile
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "none" || got[0].Servers == nil || got[0].Unknown == nil ||
		!slices.Equal(got[1].Unknown, []string{"typo-server"}) {
		t.Fatalf("%s", h.out)
	}
}

func TestMCPProfilesSaysWhenThereAreNone(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "mcp", "profiles")
	assertContains(t, h.out.String(), "No MCP profiles")
	h.out.Reset()
	h.ok(t, "mcp", "profiles", "-o", "json")
	if strings.TrimSpace(h.out.String()) != "[]" {
		t.Fatalf("%s", h.out)
	}
}
