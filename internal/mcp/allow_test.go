package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestAllowedMatchesServersByURLOrCommand(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.claudeJSON(map[string]any{"mcpServers": map[string]any{
		"browser": map[string]any{"command": "/bin/browser", "args": []string{"--headless"}},
		"web":     map[string]any{"type": "http", "url": "https://example.com/web"},
	}})
	docs := f.plugin("docs@market", `{"mcpServers": {
		"search": {"type": "http", "url": "https://example.com/mcp", "headers": {"Authorization": "sk-header-secret"}},
		"local": {"command": "${CLAUDE_PLUGIN_ROOT}/bin/server", "args": ["--root", "${CLAUDE_PLUGIN_ROOT}"]},
		"odd": {"type": "sdk"}}}`, "")

	entries, missing := f.store.Allowed(dir, []string{
		"browser", "web", "elsewhere", "plugin:docs:search", "plugin:docs:local", "plugin:docs:odd", "plugin:docs:none", "plugin:gone:x",
	})

	want := []map[string]any{
		{"serverCommand": []string{"/bin/browser", "--headless"}},
		{"serverUrl": "https://example.com/web"},
		{"serverName": "elsewhere"},
		{"serverUrl": "https://example.com/mcp"},
		{"serverCommand": []string{docs + "/bin/server", "--root", docs}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("entries\n got %v\nwant %v", entries, want)
	}
	if !reflect.DeepEqual(missing, []string{"plugin:docs:odd", "plugin:docs:none", "plugin:gone:x"}) {
		t.Fatalf("missing %q", missing)
	}
	if e, _ := f.store.Allowed(dir, nil); e == nil || len(e) != 0 {
		t.Fatalf("an empty profile allows %v", e)
	}
}

func TestSaveAllowedWritesAFileOnlyTheUserCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "profiles", "safe.json")
	entries := []map[string]any{{"serverName": "browser"}}
	if err := SaveAllowed(path, entries); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Allowed []map[string]any `json:"allowedMcpServers"`
	}
	if err := json.Unmarshal(data, &got); err != nil || len(got.Allowed) != 1 || got.Allowed[0]["serverName"] != "browser" {
		t.Fatalf("%s %v", data, err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %v", info.Mode())
	}
	if info, _ := os.Stat(filepath.Dir(path)); info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %v", info.Mode())
	}
}
