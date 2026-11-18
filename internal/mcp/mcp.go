// Package mcp reads which MCP servers Claude Code gives a directory, and
// whether each is on. It reads ~/.claude.json, the settings files, .mcp.json
// files, installed plugins, and the managed file. The one thing it writes is
// a directory's disabledMcpServers list in ~/.claude.json.
//
// A server's command, arguments, environment, headers, and URL can carry
// tokens, so this package keeps only each server's name and kind.
package mcp

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/paths"
)

// The scopes a server comes from.
const (
	User    = "user"
	Local   = "local"
	Project = "project"
	Plugin  = "plugin"
	Managed = "managed"
)

// The states of a server in a directory.
const (
	On  = "on"
	Off = "off"
	// Pending is a project server nobody has approved or rejected yet.
	Pending = "pending"
)

// Server is one MCP server as a directory sees it.
type Server struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
	State string `json:"state"`
	// Kind is stdio, http, sse, or another transport Claude Code names, or ""
	// when the entry does not say.
	Kind string `json:"kind"`
}

// Store is where Claude Code keeps MCP configuration.
type Store struct {
	// ConfigFile is ~/.claude.json.
	ConfigFile string
	// Home is ~/.claude, which holds settings.json and the plugins.
	Home string
	// Managed lists the places an administrator's managed-mcp.json can be.
	Managed []string
}

// DefaultStore is the Store for $CLAUDE_CONFIG_DIR or the home directory.
func DefaultStore() (Store, error) {
	home, err := paths.ClaudeHome()
	if err != nil {
		return Store{}, err
	}
	file, err := paths.ClaudeConfigFile()
	if err != nil {
		return Store{}, err
	}
	return Store{ConfigFile: file, Home: home, Managed: managedFiles()}, nil
}

// Servers lists the servers Claude Code gives a session in dir, an absolute
// directory. When two scopes define one name, Claude Code uses local, then
// project, then user, then plugin, and a managed file replaces them all. A
// file claudes cannot read is a problem, and the rest of the list still comes
// back.
func (s Store) Servers(dir string) ([]Server, []error) {
	var r reader
	if managed := r.managed(s.Managed); managed != nil {
		// A managed file takes exclusive control of MCP servers.
		return list(managed), r.problems
	}
	cfg := r.claudeJSON(s.ConfigFile)
	proj := cfg.project(dir)
	settings := r.settings(s.Home, dir)

	found := map[string]Server{}
	add := func(scope string, servers map[string]json.RawMessage) {
		for name, raw := range servers {
			if _, ok := found[name]; !ok {
				found[name] = Server{Name: name, Scope: scope, State: On, Kind: kindOf(raw)}
			}
		}
	}
	add(Local, objectOf(proj["mcpServers"]))
	project := r.projectServers(dir)
	add(Project, project)
	add(User, objectOf(cfg.top["mcpServers"]))
	add(Plugin, r.pluginServers(s.Home, dir, settings))

	approved, rejected := approvals(proj, settings)
	disabled := stringsOf(proj["disabledMcpServers"])
	for name, srv := range found {
		if srv.Scope == Project {
			switch {
			case slices.Contains(rejected.names, name):
				srv.State = Off
			case !approved.all && !slices.Contains(approved.names, name):
				srv.State = Pending
			}
		}
		if slices.Contains(disabled, name) {
			srv.State = Off
		}
		found[name] = srv
	}
	return list(found), r.problems
}

// Known is every server name claudes can find: dir's servers, any directory's
// local servers, every installed plugin's, and the names in any directory's
// approval and on and off lists. Only those lists record another project's
// servers, which a profile may name.
func (s Store) Known(dir string) map[string]bool {
	known := map[string]bool{}
	servers, _ := s.Servers(dir)
	for _, srv := range servers {
		known[srv.Name] = true
	}
	var r reader
	cfg := r.claudeJSON(s.ConfigFile)
	for _, raw := range objectOf(cfg.top["projects"]) {
		proj := objectOf(raw)
		for name := range objectOf(proj["mcpServers"]) {
			known[name] = true
		}
		for _, key := range []string{"disabledMcpServers", "enabledMcpServers", "enabledMcpjsonServers", "disabledMcpjsonServers"} {
			for _, name := range stringsOf(proj[key]) {
				known[name] = true
			}
		}
	}
	for name := range r.pluginServers(s.Home, dir, nil) {
		known[name] = true
	}
	return known
}

// list sorts servers on first, then pending, then off, and by name within
// each, so what a session loads reads first.
func list(found map[string]Server) []Server {
	out := make([]Server, 0, len(found))
	for _, srv := range found {
		out = append(out, srv)
	}
	rank := map[string]int{On: 0, Pending: 1, Off: 2}
	slices.SortFunc(out, func(a, b Server) int {
		return cmp.Or(cmp.Compare(rank[a.State], rank[b.State]), strings.Compare(a.Name, b.Name))
	})
	return out
}

// reader reads JSON files and collects what it could not read.
type reader struct{ problems []error }

// object reads a JSON object from path, or nil when the file is missing or is
// not an object. A relative path comes from a Store with a field unset, and
// would read the current directory, so it reads nothing.
func (r *reader) object(path string) map[string]json.RawMessage {
	if !filepath.IsAbs(path) {
		return nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		r.problems = append(r.problems, err)
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		r.problems = append(r.problems, fmt.Errorf("%s: %w", path, err))
		return nil
	}
	return m
}

// claudeConfig is ~/.claude.json.
type claudeConfig struct{ top map[string]json.RawMessage }

func (r *reader) claudeJSON(path string) claudeConfig {
	return claudeConfig{top: r.object(path)}
}

// project is the entry ~/.claude.json keeps for dir.
func (c claudeConfig) project(dir string) map[string]json.RawMessage {
	key, ok := c.projectKey(dir)
	if !ok {
		return nil
	}
	return objectOf(objectOf(c.top["projects"])[key])
}

// configKey is a directory as ~/.claude.json's projects name it, with forward
// slashes, which Claude Code writes on Windows too.
func configKey(dir string) string { return filepath.ToSlash(filepath.Clean(dir)) }

// settingsFiles are the user's settings.json, then the project's committed
// and local settings in dir, each overriding the one before.
type settingsFiles []map[string]json.RawMessage

func (r *reader) settings(home, dir string) settingsFiles {
	return settingsFiles{
		r.object(filepath.Join(home, "settings.json")),
		r.object(filepath.Join(dir, ".claude", "settings.json")),
		r.object(filepath.Join(dir, ".claude", "settings.local.json")),
	}
}

// projectServers reads .mcp.json in dir and each directory above it. A closer
// file wins a name both define.
func (r *reader) projectServers(dir string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for d := dir; ; {
		for name, raw := range serversIn(r.object(filepath.Join(d, ".mcp.json"))) {
			if _, ok := out[name]; !ok {
				out[name] = raw
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			return out
		}
		d = parent
	}
}

// approval is the project servers a directory approved or rejected.
type approval struct {
	all   bool
	names []string
}

// approvals gathers the project server approvals from the directory's entry in
// ~/.claude.json and from every settings file.
func approvals(proj map[string]json.RawMessage, settings settingsFiles) (approved, rejected approval) {
	for _, m := range append(settingsFiles{proj}, settings...) {
		approved.all = approved.all || boolOf(m["enableAllProjectMcpServers"])
		approved.names = append(approved.names, stringsOf(m["enabledMcpjsonServers"])...)
		rejected.names = append(rejected.names, stringsOf(m["disabledMcpjsonServers"])...)
	}
	return approved, rejected
}

// pluginServers reads the servers of the plugins the settings enable, named
// plugin:PLUGIN:SERVER as Claude Code names them. With no settings, it reads
// every installed plugin's.
func (r *reader) pluginServers(home, dir string, settings settingsFiles) map[string]json.RawMessage {
	installed := objectOf(r.object(filepath.Join(home, "plugins", "installed_plugins.json"))["plugins"])
	var ids []string
	if settings == nil {
		for id := range installed {
			ids = append(ids, id)
		}
	} else {
		enabled := map[string]bool{}
		for _, m := range settings {
			for id, on := range enabledPlugins(m["enabledPlugins"]) {
				enabled[id] = on
			}
		}
		for id, on := range enabled {
			if on {
				ids = append(ids, id)
			}
		}
	}
	out := map[string]json.RawMessage{}
	for _, id := range ids {
		plugin, marketplace, ok := strings.Cut(id, "@")
		if !ok || plugin == "" {
			continue
		}
		root := installPath(installed[id], dir)
		if root == "" {
			root = newestDir(filepath.Join(home, "plugins", "cache", marketplace, plugin))
		}
		if root == "" {
			continue
		}
		for name, raw := range r.pluginConfig(root) {
			out["plugin:"+plugin+":"+name] = raw
		}
	}
	return out
}

// pluginConfig reads a plugin's servers: those in its .mcp.json, and those its
// plugin.json lists inline or names files for.
func (r *reader) pluginConfig(root string) map[string]json.RawMessage {
	out := serversIn(r.object(filepath.Join(root, ".mcp.json")))
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	manifest := r.object(filepath.Join(root, ".claude-plugin", "plugin.json"))
	var entries []json.RawMessage
	if err := json.Unmarshal(manifest["mcpServers"], &entries); err != nil {
		entries = []json.RawMessage{manifest["mcpServers"]}
	}
	for _, e := range entries {
		var servers map[string]json.RawMessage
		var file string
		switch {
		case json.Unmarshal(e, &file) == nil && file != "":
			if filepath.IsAbs(file) {
				continue
			}
			servers = serversIn(r.object(filepath.Join(root, filepath.FromSlash(file))))
		default:
			servers = serversIn(objectOf(e))
		}
		for name, raw := range servers {
			out[name] = raw
		}
	}
	return out
}

// installPath is where installed_plugins.json says a plugin is installed for
// dir: an install for every directory, or one for dir itself.
func installPath(raw json.RawMessage, dir string) string {
	var installs []json.RawMessage
	if err := json.Unmarshal(raw, &installs); err != nil {
		installs = []json.RawMessage{raw}
	}
	for _, i := range installs {
		var in struct {
			InstallPath any `json:"installPath"`
			ProjectPath any `json:"projectPath"`
		}
		if json.Unmarshal(i, &in) != nil {
			continue
		}
		path, _ := in.InstallPath.(string)
		project, _ := in.ProjectPath.(string)
		if filepath.IsAbs(path) && (project == "" || configKey(project) == configKey(dir)) {
			return path
		}
	}
	return ""
}

// newestDir is the most recently changed directory in dir, such as the newest
// version of a plugin in the cache, or "".
func newestDir(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestInfo fs.FileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !e.IsDir() {
			continue
		}
		if newestInfo == nil || info.ModTime().After(newestInfo.ModTime()) {
			newest, newestInfo = filepath.Join(dir, e.Name()), info
		}
	}
	return newest
}

// enabledPlugins reads enabledPlugins: an object of plugin@marketplace to
// true or false, or a list of the enabled ones.
func enabledPlugins(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	for id, v := range objectOf(raw) {
		out[id] = boolOf(v)
	}
	for _, id := range stringsOf(raw) {
		out[id] = true
	}
	return out
}

// managed reads the first managed-mcp.json that is there, or nil.
func (r *reader) managed(files []string) map[string]Server {
	for _, f := range files {
		m := r.object(f)
		if m == nil {
			continue
		}
		out := map[string]Server{}
		for name, raw := range serversIn(m) {
			out[name] = Server{Name: name, Scope: Managed, State: On, Kind: kindOf(raw)}
		}
		return out
	}
	return nil
}

// serversIn is the servers a .mcp.json-style file holds: under mcpServers, or
// at the top level, as some plugins write it.
func serversIn(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return nil
	}
	if raw, ok := m["mcpServers"]; ok {
		return objectOf(raw)
	}
	out := map[string]json.RawMessage{}
	for name, raw := range m {
		if objectOf(raw) != nil {
			out[name] = raw
		}
	}
	return out
}

var transport = regexp.MustCompile(`^[a-z][a-z0-9-]{0,15}$`)

// kindOf is a server's transport. An entry with no type and a command is
// stdio, as Claude Code reads it.
func kindOf(raw json.RawMessage) string {
	m := objectOf(raw)
	var kind string
	if json.Unmarshal(m["type"], &kind) == nil && transport.MatchString(kind) {
		return kind
	}
	if _, ok := m["type"]; !ok && m["command"] != nil {
		return "stdio"
	}
	return ""
}

// objectOf decodes a JSON object, or nil when raw is not one.
func objectOf(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

// stringsOf decodes the strings in a JSON list, skipping anything else.
func stringsOf(raw json.RawMessage) []string {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var out []string
	for _, item := range items {
		var s string
		if json.Unmarshal(item, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}

func boolOf(raw json.RawMessage) bool {
	var b bool
	return json.Unmarshal(raw, &b) == nil && b
}

// gitRoot is the root of the main worktree of the git repository dir is in,
// or "" outside one. A linked worktree's .git file names its git directory,
// whose commondir file leads to the main one.
func gitRoot(dir string) string {
	for d := dir; ; {
		dotGit := filepath.Join(d, ".git")
		info, err := os.Stat(dotGit)
		switch {
		case err == nil && info.IsDir():
			return d
		case err == nil:
			return linkedRoot(d, dotGit)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// linkedRoot follows a .git file in dir. A submodule's has no commondir, so
// the submodule is its own root.
func linkedRoot(dir, dotGit string) string {
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return dir
	}
	gitDir, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
	if !ok {
		return dir
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return dir
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return filepath.Dir(filepath.Clean(commonDir))
}
