package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
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

// docsPlugin installs a plugin whose server carries a secret in its URL.
func (h *harness) docsPlugin(t *testing.T) {
	t.Helper()
	root := filepath.Join(h.root, "plugins", "cache", "market", "docs", "1.0.0")
	writeFile(t, filepath.Join(root, ".mcp.json"), `{"mcpServers": {"search": {"type": "http", "url": "https://example.com/sk-url-secret"}}}`)
	index, _ := json.Marshal(map[string]any{"version": 2, "plugins": map[string]any{
		"docs@market": []map[string]any{{"scope": "user", "installPath": root}},
	}})
	writeFile(t, filepath.Join(h.root, "plugins", "installed_plugins.json"), string(index))
}

func TestAgentsWithAProfileAllowsExactlyItsServers(t *testing.T) {
	h := newHarness(t)
	h.docsPlugin(t)
	h.mcpServers(t, h.folder("api"), map[string]any{})
	h.withConfig(t, "mcp:\n  profiles:\n    safe: [browser, \"plugin:docs:search\"]\n")

	h.ok(t, "agents", "--profile", "safe", "--model", "opus")

	file := filepath.Join(h.root, "state", "profiles", "safe.json")
	want := []string{"claude", "agents", "--settings", file, "--model", "opus"}
	if len(h.execs) != 1 || !slices.Equal(h.execs[0], want) {
		t.Fatalf("execs %q", h.execs)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Allowed []map[string]any `json:"allowedMcpServers"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Allowed) != 2 || got.Allowed[0]["serverCommand"] == nil || got.Allowed[1]["serverUrl"] != "https://example.com/sk-url-secret" {
		t.Fatalf("%s", data)
	}
	noSecrets(t, h.out.String()+h.err.String())
	if info, _ := os.Stat(file); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}

	h.execs = nil
	h.ok(t, "agents", "--profile=safe")
	if len(h.execs) != 1 || !slices.Equal(h.execs[0], want[:4]) {
		t.Fatalf("execs %q", h.execs)
	}
}

func TestAgentsWithoutAProfilePassesEverythingThrough(t *testing.T) {
	for _, args := range [][]string{{"agents"}, {"agents", "--cwd", "."}, {"agents", "--", "--profile", "x"}} {
		h := newHarness(t)
		h.ok(t, args...)
		want := append([]string{"claude"}, args...)
		if len(h.execs) != 1 || !slices.Equal(h.execs[0], want) {
			t.Fatalf("%q: execs %q", args, h.execs)
		}
	}
	h := newHarness(t)
	h.ok(t, "agents", "--help")
	if len(h.execs) != 0 || !strings.Contains(h.out.String(), "--profile NAME") {
		t.Fatalf("execs %q\n%s", h.execs, h.out)
	}
}

func TestAgentsRefusesWhatAProfileCannotDo(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "mcp:\n  profiles:\n    safe: [browser]\n    plugged: [\"plugin:gone:x\"]\n")
	for args, want := range map[string]string{
		"agents --profile":                   "needs a profile name",
		"agents --profile=":                  "needs a profile name",
		"agents --profile nope":              "Profiles: plugged, safe",
		"agents --profile safe --settings x": "one or the other",
		"agents --profile safe --settings=x": "one or the other",
		"agents --profile plugged":           "plugin:gone:x",
	} {
		h.err.Reset()
		h.fails(t, strings.Fields(args)...)
		assertContains(t, h.err.String(), want)
	}
	if len(h.execs) != 0 {
		t.Fatalf("execs %q", h.execs)
	}
}
