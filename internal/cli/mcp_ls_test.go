package cli

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

// secrets are what a server entry can carry that claudes must never print.
var secrets = []string{"sk-command-secret", "sk-arg-secret", "sk-env-secret", "sk-url-secret", "sk-header-secret"}

// mcpServers writes ~/.claude.json with made-up user servers that carry
// secrets, and dir's local entry.
func (h *harness) mcpServers(t *testing.T, dir string, project map[string]any) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"browser": map[string]any{"command": "/bin/sk-command-secret", "args": []string{"--token", "sk-arg-secret"}, "env": map[string]string{"TOKEN": "sk-env-secret"}},
			"web-api": map[string]any{"type": "http", "url": "https://example.com/sk-url-secret", "headers": map[string]string{"Authorization": "sk-header-secret"}},
		},
		"projects": map[string]any{filepath.ToSlash(dir): project},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.app.MCP.ConfigFile, string(data))
}

func noSecrets(t *testing.T, text string) {
	t.Helper()
	for _, s := range secrets {
		if strings.Contains(text, s) {
			t.Fatalf("printed %s:\n%s", s, text)
		}
	}
}

func TestMCPLsShowsNameScopeStateAndKindOnly(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("api")
	h.mcpServers(t, dir, map[string]any{"disabledMcpServers": []string{"web-api"}})
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": {"repo-tool": {"command": "x", "env": {"KEY": "sk-env-secret"}}}}`)

	h.ok(t, "mcp", "ls", "--in", dir)
	text := h.out.String()
	noSecrets(t, text)
	want := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^MCP servers in .*api:$`),
		regexp.MustCompile(`(?m)^NAME\s+SCOPE\s+STATE\s+KIND\s+PROFILES$`),
		regexp.MustCompile(`(?m)^browser\s+user\s+on\s+stdio\s+-$`),
		regexp.MustCompile(`(?m)^repo-tool\s+project\s+pending\s+stdio\s+-$`),
		regexp.MustCompile(`(?m)^web-api\s+user\s+off\s+http\s+-$`),
	}
	for _, re := range want {
		if !re.MatchString(text) {
			t.Fatalf("no line matches %s:\n%s", re, text)
		}
	}
	if strings.Index(text, "browser") > strings.Index(text, "repo-tool") || strings.Index(text, "repo-tool") > strings.Index(text, "web-api") {
		t.Fatalf("want on, then pending, then off:\n%s", text)
	}

	h.out.Reset()
	h.ok(t, "mcp", "ls", "--in", dir, "-o", "json")
	noSecrets(t, h.out.String())
	var got struct {
		Dir     string           `json:"dir"`
		Profile *string          `json:"profile"`
		Servers []map[string]any `json:"servers"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	if got.Dir != dir || got.Profile == nil || len(got.Servers) != 3 {
		t.Fatalf("%s", h.out)
	}
	for _, s := range got.Servers {
		keys := []string{}
		for k := range s {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		if !slices.Equal(keys, []string{"kind", "name", "profileState", "profiles", "scope", "state"}) {
			t.Fatalf("keys %v in %s", keys, h.out)
		}
		if p, ok := s["profiles"].([]any); !ok || len(p) != 0 {
			t.Fatalf("profiles is not an empty list: %s", h.out)
		}
	}
}

// A warning about a file claudes cannot read goes to stderr, so it must not
// quote the file either.
func TestMCPLsPrintsNoSecretOnStderr(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("api")
	h.mcpServers(t, dir, map[string]any{})
	h.ok(t, "mcp", "ls", "--in", dir)
	noSecrets(t, h.out.String()+h.err.String())

	for _, broken := range []struct{ path, body string }{
		{filepath.Join(dir, ".mcp.json"), `{"mcpServers": {"repo-tool": {"command": "/bin/sk-command-secret", "env": {"KEY": "sk-env-secret"}`},
		{filepath.Join(dir, ".mcp.json"), `{"mcpServers": {"repo-tool": {"args": ["sk-arg-secret"], "url": sk-url-secret}}}`},
		{h.app.MCP.ConfigFile, `{"mcpServers": {"browser": {"headers": {"Authorization": "sk-header-secret"}}} sk-env-secret}`},
	} {
		writeFile(t, broken.path, broken.body)
		for _, format := range []string{"text", "json"} {
			h.out.Reset()
			h.err.Reset()
			h.run("mcp", "ls", "--in", dir, "-o", format)
			assertContains(t, h.err.String(), "cannot read every MCP setting")
			noSecrets(t, h.out.String()+h.err.String())
		}
	}
}

func TestMCPLsTakesASessionOrADirectory(t *testing.T) {
	h := newHarness(t)
	api, docs := h.folder("api"), h.folder("docs")
	writeFile(t, filepath.Join(api, ".mcp.json"), `{"mcpServers": {"api-tool": {"type": "sse", "url": "u"}}}`)
	writeFile(t, filepath.Join(docs, ".mcp.json"), `{"mcpServers": {"docs-tool": {"command": "x"}}}`)
	h.transcript(t, "api-work", api, time.Hour)

	h.ok(t, "mcp", "ls", "api-work")
	assertContains(t, h.out.String(), "api-tool")
	if strings.Contains(h.out.String(), "docs-tool") {
		t.Fatalf("listed another directory's server:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "mcp", "ls", "-i", docs)
	assertContains(t, h.out.String(), "docs-tool")

	h.out.Reset()
	t.Chdir(docs)
	h.ok(t, "mcp", "ls")
	assertContains(t, h.out.String(), "docs-tool")

	h.fails(t, "mcp", "ls", "api-work", "--in", docs)
	assertContains(t, h.err.String(), "not both")
	h.exits(t, exitNoMatch, "mcp", "ls", "nobody")
	h.fails(t, "mcp", "ls", "--in", filepath.Join(docs, "missing"))
}

// mcp ls --in and session mv DIR resolve a directory the same way, and say so
// when it does not exist rather than print stat's error.
func TestMCPLsAndSessionMvSayADirectoryDoesNotExist(t *testing.T) {
	h := newHarness(t)
	missing := filepath.Join(h.folder("docs"), "missing")
	h.transcript(t, "api-work", h.folder("api"), time.Hour)
	h.agents.snapshots = [][]claude.Session{{}}

	h.fails(t, "mcp", "ls", "--in", missing)
	assertContains(t, h.err.String(), missing+" does not exist")
	if strings.Contains(h.err.String(), "stat ") {
		t.Fatalf("printed stat's error: %s", h.err)
	}
	h.err.Reset()
	h.fails(t, "session", "mv", "api-work", missing)
	assertContains(t, h.err.String(), missing+" does not exist. Create or clone it first")
}

func TestMCPLsSaysWhenThereAreNone(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "mcp", "ls", "--in", h.folder("empty"))
	assertContains(t, h.out.String(), "No MCP servers in ")
}

func TestMCPLsWarnsAboutABrokenFileAndListsTheRest(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("api")
	h.mcpServers(t, dir, map[string]any{})
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers": `)
	h.ok(t, "mcp", "ls", "--in", dir)
	assertContains(t, h.out.String(), "browser")
	assertContains(t, h.err.String(), "cannot read every MCP setting")
}

func TestMCPLsShowsProfiles(t *testing.T) {
	h := newHarness(t)
	dir := h.folder("api")
	h.mcpServers(t, dir, map[string]any{"disabledMcpServers": []string{"web-api"}})
	h.withConfig(t, "mcp:\n  profiles:\n    safe: [browser]\n    web: [browser, web-api]\n")

	h.ok(t, "mcp", "ls", "--in", dir)
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^browser\s+user\s+on\s+stdio\s+safe, web$`),
		regexp.MustCompile(`(?m)^web-api\s+user\s+off\s+http\s+web$`),
	} {
		if !re.MatchString(h.out.String()) {
			t.Fatalf("no line matches %s:\n%s", re, h.out)
		}
	}

	h.out.Reset()
	h.ok(t, "mcp", "ls", "--in", dir, "--profile", "safe")
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)under profile safe:$`),
		regexp.MustCompile(`(?m)^NAME\s+SCOPE\s+STATE\s+PROFILE\s+KIND\s+PROFILES$`),
		regexp.MustCompile(`(?m)^browser\s+user\s+on\s+on\s+stdio`),
		regexp.MustCompile(`(?m)^web-api\s+user\s+off\s+off\s+http`),
	} {
		if !re.MatchString(h.out.String()) {
			t.Fatalf("no line matches %s:\n%s", re, h.out)
		}
	}

	h.out.Reset()
	h.ok(t, "mcp", "ls", "--in", dir, "--profile", "web", "-o", "json")
	assertContains(t, h.out.String(), `"profile": "web"`)
	var got struct {
		Servers []struct {
			Name, State, ProfileState string
		} `json:"servers"`
	}
	if err := json.Unmarshal(h.out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, s := range got.Servers {
		if s.ProfileState != "on" {
			t.Fatalf("%s would be %q under web: %s", s.Name, s.ProfileState, h.out)
		}
	}

	h.fails(t, "mcp", "ls", "--in", dir, "--profile", "nope")
	assertContains(t, h.err.String(), "Profiles: safe, web")
	if got := h.complete(t, "mcp", "ls", "--profile", ""); !slices.Equal(got, []string{"safe", "web"}) {
		t.Fatalf("completion: %v", got)
	}
}

func TestMCPLsWithoutProfilesRefusesOne(t *testing.T) {
	h := newHarness(t)
	h.fails(t, "mcp", "ls", "--in", h.folder("api"), "--profile", "safe")
	assertContains(t, h.err.String(), "no mcp.profiles")
}
