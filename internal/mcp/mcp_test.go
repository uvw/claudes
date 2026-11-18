package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

// fixture is a home directory with a ~/.claude.json, a ~/.claude, and a work
// directory, all made up.
type fixture struct {
	t     *testing.T
	root  string
	store Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, root: root, store: Store{
		ConfigFile: filepath.Join(root, ".claude.json"),
		Home:       filepath.Join(root, ".claude"),
	}}
}

func (f *fixture) dir(parts ...string) string {
	f.t.Helper()
	d := filepath.Join(append([]string{f.root, "work"}, parts...)...)
	if err := os.MkdirAll(d, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return d
}

func (f *fixture) write(path, body string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// claudeJSON writes ~/.claude.json from v, with projects keyed by directory.
func (f *fixture) claudeJSON(v map[string]any) {
	f.t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(f.store.ConfigFile, string(data))
}

// servers lists dir's servers as "name scope state kind" and fails on any
// problem.
func (f *fixture) servers(dir string) []string {
	f.t.Helper()
	list, problems := f.store.Servers(dir)
	if len(problems) > 0 {
		f.t.Fatalf("problems: %v", problems)
	}
	return lines(list)
}

func lines(list []Server) []string {
	out := []string{}
	for _, s := range list {
		out = append(out, strings.Join([]string{s.Name, s.Scope, s.State, s.Kind}, " "))
	}
	return out
}

func same(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestUserServersAndTheirKinds(t *testing.T) {
	f := newFixture(t)
	f.claudeJSON(map[string]any{"mcpServers": map[string]any{
		"local-tool":   map[string]any{"command": "tool", "args": []string{"--token", "secret"}},
		"typed-stdio":  map[string]any{"type": "stdio", "command": "tool"},
		"web-api":      map[string]any{"type": "http", "url": "https://example.com/mcp"},
		"stream":       map[string]any{"type": "sse", "url": "https://example.com/sse"},
		"odd-type":     map[string]any{"type": 7, "command": "tool"},
		"strange-type": map[string]any{"type": "Not A Type!"},
		"no-fields":    map[string]any{},
	}})
	same(t, f.servers(f.dir()), []string{
		"local-tool user on stdio",
		"no-fields user on ",
		"odd-type user on ",
		"strange-type user on ",
		"stream user on sse",
		"typed-stdio user on stdio",
		"web-api user on http",
	})
}

func TestNothingToReadIsNoServersAndNoProblem(t *testing.T) {
	f := newFixture(t)
	same(t, f.servers(f.dir()), []string{})
	f.store = Store{}
	same(t, f.servers(f.dir()), []string{})
}

func TestABrokenFileIsAProblemAndTheRestStillLists(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.claudeJSON(map[string]any{"mcpServers": map[string]any{"helper": map[string]any{"command": "x"}}})
	f.write(filepath.Join(dir, ".mcp.json"), `{"mcpServers": `)
	list, problems := f.store.Servers(dir)
	if len(problems) != 1 || !strings.Contains(problems[0].Error(), ".mcp.json") {
		t.Fatalf("problems: %v", problems)
	}
	same(t, lines(list), []string{"helper user on stdio"})
}

func TestOddShapesAreSkipped(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.write(f.store.ConfigFile, `{"mcpServers": ["helper"], "projects": {"`+filepath.ToSlash(dir)+`": {"mcpServers": {"kept": {"command": "x"}}, "disabledMcpServers": "kept"}}}`)
	same(t, f.servers(dir), []string{"kept local on stdio"})
}

func TestScopesAndPrecedence(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.claudeJSON(map[string]any{
		"mcpServers": map[string]any{
			"shared":    map[string]any{"type": "http", "url": "u"},
			"user-only": map[string]any{"command": "x"},
			"twice":     map[string]any{"command": "x"},
		},
		"projects": map[string]any{filepath.ToSlash(dir): map[string]any{
			"mcpServers": map[string]any{"shared": map[string]any{"type": "sse", "url": "u"}},
		}},
	})
	f.write(filepath.Join(dir, ".mcp.json"), `{"mcpServers": {"shared": {"command": "x"}, "twice": {"type": "http", "url": "u"}}}`)
	same(t, f.servers(dir), []string{
		"shared local on sse",
		"user-only user on stdio",
		"twice project pending http",
	})
}

func TestProjectApprovals(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.write(filepath.Join(dir, ".mcp.json"), `{"mcpServers": {"asked": {"command": "x"}, "yes": {"command": "x"}, "no": {"command": "x"}, "said-yes": {"command": "x"}}}`)
	f.claudeJSON(map[string]any{"projects": map[string]any{filepath.ToSlash(dir): map[string]any{
		"enabledMcpjsonServers":  []string{"yes"},
		"disabledMcpjsonServers": []string{"no"},
	}}})
	f.write(filepath.Join(dir, ".claude", "settings.json"), `{"enabledMcpjsonServers": ["said-yes"]}`)
	same(t, f.servers(dir), []string{
		"said-yes project on stdio",
		"yes project on stdio",
		"asked project pending stdio",
		"no project off stdio",
	})

	f.write(filepath.Join(dir, ".claude", "settings.local.json"), `{"enableAllProjectMcpServers": true}`)
	same(t, f.servers(dir), []string{
		"asked project on stdio",
		"said-yes project on stdio",
		"yes project on stdio",
		"no project off stdio",
	})
}

func TestProjectServersComeFromAboveTooAndTheCloserFileWins(t *testing.T) {
	f := newFixture(t)
	top, sub := f.dir("repo"), f.dir("repo", "pkg")
	f.write(filepath.Join(top, ".mcp.json"), `{"mcpServers": {"both": {"type": "http", "url": "u"}, "above": {"command": "x"}}}`)
	f.write(filepath.Join(sub, ".mcp.json"), `{"mcpServers": {"both": {"command": "x"}}}`)
	same(t, f.servers(sub), []string{"above project pending stdio", "both project pending stdio"})
}

func TestDisabledServersAreOffInTheirDirectoryOnly(t *testing.T) {
	f := newFixture(t)
	dir, other := f.dir("app"), f.dir("other")
	f.claudeJSON(map[string]any{
		"mcpServers": map[string]any{"helper": map[string]any{"command": "x"}, "browser": map[string]any{"command": "x"}},
		"projects": map[string]any{filepath.ToSlash(dir): map[string]any{
			"disabledMcpServers": []string{"helper", "plugin:docs:search"},
			// Opting in again matters only for a server that starts off.
			"enabledMcpServers": []string{"helper"},
		}},
	})
	f.plugin("docs@market", `{"search": {"type": "http", "url": "u"}}`, "")
	f.write(filepath.Join(f.store.Home, "settings.json"), `{"enabledPlugins": {"docs@market": true}}`)
	same(t, f.servers(dir), []string{
		"browser user on stdio",
		"helper user off stdio",
		"plugin:docs:search plugin off http",
	})
	same(t, f.servers(other), []string{
		"browser user on stdio",
		"helper user on stdio",
		"plugin:docs:search plugin on http",
	})
}

// plugin installs id, as plugin@marketplace, with mcp as its .mcp.json and
// manifest as its plugin.json, either "" for none, and records it in
// installed_plugins.json.
func (f *fixture) plugin(id, mcp, manifest string) string {
	f.t.Helper()
	name, market, _ := strings.Cut(id, "@")
	root := filepath.Join(f.store.Home, "plugins", "cache", market, name, "1.0.0")
	if err := os.MkdirAll(root, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if mcp != "" {
		f.write(filepath.Join(root, ".mcp.json"), mcp)
	}
	if manifest != "" {
		f.write(filepath.Join(root, ".claude-plugin", "plugin.json"), manifest)
	}
	index := filepath.Join(f.store.Home, "plugins", "installed_plugins.json")
	installed := map[string]any{"version": 2, "plugins": map[string]any{}}
	if data, err := os.ReadFile(index); err == nil {
		if err := json.Unmarshal(data, &installed); err != nil {
			f.t.Fatal(err)
		}
	}
	installed["plugins"].(map[string]any)[id] = []map[string]any{{"scope": "user", "installPath": root, "version": "1.0.0"}}
	data, err := json.Marshal(installed)
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(index, string(data))
	return root
}

func TestPluginServersOfEnabledPluginsOnly(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	f.plugin("docs@market", `{"mcpServers": {"search": {"type": "http", "url": "u"}}}`, "")
	f.plugin("flat@market", `{"tool": {"command": "x"}}`, "")
	f.plugin("off@market", `{"mcpServers": {"hidden": {"command": "x"}}}`, "")
	f.plugin("inline@market", "", `{"name": "inline", "mcpServers": {"embedded": {"command": "x"}}}`)
	listed := f.plugin("listed@market", "", `{"name": "listed", "mcpServers": ["./servers.json", "/elsewhere/servers.json"]}`)
	f.write(filepath.Join(listed, "servers.json"), `{"mcpServers": {"from-file": {"type": "sse", "url": "u"}}}`)
	f.write(filepath.Join(f.store.Home, "settings.json"), `{"enabledPlugins": {
		"docs@market": true, "flat@market": true, "off@market": false,
		"inline@market": true, "listed@market": true, "later@market": true, "gone@market": true}}`)
	f.write(filepath.Join(dir, ".claude", "settings.json"), `{"enabledPlugins": {"later@market": false}}`)
	// A plugin missing from installed_plugins.json is read from its newest
	// version in the cache.
	f.write(filepath.Join(f.store.Home, "plugins", "cache", "market", "later", "2.0.0", ".mcp.json"), `{"cached": {"command": "x"}}`)

	same(t, f.servers(dir), []string{
		"plugin:docs:search plugin on http",
		"plugin:flat:tool plugin on stdio",
		"plugin:inline:embedded plugin on stdio",
		"plugin:listed:from-file plugin on sse",
	})
	same(t, f.servers(f.dir("other")), []string{
		"plugin:docs:search plugin on http",
		"plugin:flat:tool plugin on stdio",
		"plugin:inline:embedded plugin on stdio",
		"plugin:later:cached plugin on stdio",
		"plugin:listed:from-file plugin on sse",
	})
}

func TestAListOfEnabledPluginsCountsToo(t *testing.T) {
	f := newFixture(t)
	f.plugin("docs@market", `{"search": {"type": "http", "url": "u"}}`, "")
	f.write(filepath.Join(f.store.Home, "settings.json"), `{"enabledPlugins": ["docs@market", "bad-id"]}`)
	same(t, f.servers(f.dir()), []string{"plugin:docs:search plugin on http"})
}

func TestAProjectInstallAppliesToItsDirectoryOnly(t *testing.T) {
	f := newFixture(t)
	dir := f.dir("app")
	root := f.dir("plugin-copy")
	f.write(filepath.Join(root, ".mcp.json"), `{"search": {"type": "http", "url": "u"}}`)
	f.write(filepath.Join(f.store.Home, "plugins", "installed_plugins.json"),
		`{"version": 2, "plugins": {"docs@market": [{"scope": "project", "projectPath": "`+filepath.ToSlash(dir)+`", "installPath": "`+filepath.ToSlash(root)+`"}]}}`)
	f.write(filepath.Join(f.store.Home, "settings.json"), `{"enabledPlugins": {"docs@market": true}}`)
	same(t, f.servers(dir), []string{"plugin:docs:search plugin on http"})
	same(t, f.servers(f.dir("other")), []string{})
}

func TestAManagedFileTakesOver(t *testing.T) {
	f := newFixture(t)
	f.claudeJSON(map[string]any{"mcpServers": map[string]any{"helper": map[string]any{"command": "x"}}})
	managed := filepath.Join(f.root, "managed", "managed-mcp.json")
	f.store.Managed = []string{filepath.Join(f.root, "missing.json"), managed}
	same(t, f.servers(f.dir()), []string{"helper user on stdio"})
	f.write(managed, `{"mcpServers": {"company": {"type": "http", "url": "u"}}}`)
	same(t, f.servers(f.dir()), []string{"company managed on http"})
}

func TestKnownCoversOtherDirectoriesAndDisabledPlugins(t *testing.T) {
	f := newFixture(t)
	f.claudeJSON(map[string]any{
		"mcpServers": map[string]any{"helper": map[string]any{"command": "x"}},
		"projects": map[string]any{
			"/elsewhere/a": map[string]any{"mcpServers": map[string]any{"their-local": map[string]any{"command": "x"}}},
			"/elsewhere/b": map[string]any{"enabledMcpjsonServers": []string{"their-project"}, "disabledMcpServers": []string{"plugin:x:y"}},
		},
	})
	f.plugin("off@market", `{"hidden": {"command": "x"}}`, "")
	known := f.store.Known(f.dir())
	for _, name := range []string{"helper", "their-local", "their-project", "plugin:x:y", "plugin:off:hidden"} {
		if !known[name] {
			t.Errorf("%s is not known: %v", name, known)
		}
	}
	if known["nobody"] {
		t.Error("an unknown name is known")
	}
}

func TestKnownReadsClaudeJSONOnce(t *testing.T) {
	f := newFixture(t)
	f.claudeJSON(map[string]any{"mcpServers": map[string]any{"helper": map[string]any{"command": "x"}}})
	reads := 0
	readConfig = func(path string) (claude.Config, error) {
		if path == f.store.ConfigFile {
			reads++
		}
		return claude.ReadConfig(path)
	}
	t.Cleanup(func() { readConfig = claude.ReadConfig })
	if !f.store.Known(f.dir())["helper"] {
		t.Fatal("helper is not known")
	}
	if reads != 1 {
		t.Fatalf("read %s %d times", f.store.ConfigFile, reads)
	}
}

func TestManagedFilesAreThisPlatformsOnly(t *testing.T) {
	want := map[string]string{
		"darwin":  "/Library/Application Support/ClaudeCode/managed-mcp.json",
		"linux":   "/etc/claude-code/managed-mcp.json",
		"windows": `C:\Program Files\ClaudeCode\managed-mcp.json`,
	}[runtime.GOOS]
	if want == "" {
		t.Skip("no managed file path to check on " + runtime.GOOS)
	}
	t.Setenv("ProgramFiles", `C:\Program Files`)
	if got := managedFiles(); len(got) != 1 || got[0] != want {
		t.Fatalf("managed files %q, want only %q", got, want)
	}
}

func TestDefaultStoreFollowsClaudeConfigDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	t.Setenv("XDG_STATE_HOME", dir)
	s, err := DefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	if s.Home != dir || s.ConfigFile != filepath.Join(dir, ".claude.json") || len(s.Managed) == 0 ||
		s.Profiles != filepath.Join(dir, "claudes", "profiles") {
		t.Fatalf("%+v", s)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "relative")
	if _, err := DefaultStore(); err == nil {
		t.Fatal("accepted a relative CLAUDE_CONFIG_DIR")
	}
}
