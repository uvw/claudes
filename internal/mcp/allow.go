package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/uvw/claudes/internal/safefile"
)

// Allowed is the allowedMcpServers setting that keeps a session started in
// dir to exactly the servers names lists. Claude Code matches a plugin's
// server only by its URL or command, and once the list holds one command, a
// stdio server only by its command; a URL does the same for remote servers.
// So each server claudes can find goes in by its serverUrl, or its
// serverCommand with ${CLAUDE_PLUGIN_ROOT} filled in, and any other by its
// serverName. missing lists the plugin servers claudes found no URL or
// command for. The entries can hold tokens, so nothing prints them.
func (s Store) Allowed(dir string, names []string) (entries []map[string]any, missing []string) {
	var r reader
	type found struct {
		raw  json.RawMessage
		root string
	}
	known := map[string]found{}
	add := func(servers map[string]json.RawMessage, root string, prefix string) {
		for name, raw := range servers {
			if _, ok := known[prefix+name]; !ok {
				known[prefix+name] = found{raw, root}
			}
		}
	}
	cfg := r.claudeJSON(s.ConfigFile)
	add(objectOf(cfg.project(dir)["mcpServers"]), "", "")
	add(r.projectServers(dir), "", "")
	add(objectOf(cfg.top["mcpServers"]), "", "")
	for _, p := range r.plugins(s.Home, dir, nil) {
		add(r.pluginConfig(p.root), p.root, "plugin:"+p.name+":")
	}
	entries = []map[string]any{}
	for _, name := range names {
		f, ok := known[name]
		var entry map[string]any
		if ok {
			entry = matcher(objectOf(f.raw), f.root)
		}
		switch {
		case entry != nil:
			entries = append(entries, entry)
		case strings.HasPrefix(name, "plugin:"):
			missing = append(missing, name)
		default:
			entries = append(entries, map[string]any{"serverName": name})
		}
	}
	return entries, missing
}

// matcher is how allowedMcpServers names one server: by its URL, or by its
// command and arguments, or nil when it has neither.
func matcher(cfg map[string]json.RawMessage, root string) map[string]any {
	var url, command string
	if json.Unmarshal(cfg["url"], &url) == nil && url != "" {
		return map[string]any{"serverUrl": url}
	}
	if json.Unmarshal(cfg["command"], &command) != nil || command == "" {
		return nil
	}
	argv := []string{command}
	var args []string
	if err := json.Unmarshal(cfg["args"], &args); err == nil {
		argv = append(argv, args...)
	}
	if root != "" {
		for i, a := range argv {
			argv[i] = strings.ReplaceAll(a, "${CLAUDE_PLUGIN_ROOT}", root)
		}
	}
	return map[string]any{"serverCommand": argv}
}

// SaveAllowed writes a settings file holding allowedMcpServers, for
// claude --settings, readable only by the user, since the entries can hold
// tokens.
func SaveAllowed(path string, entries []map[string]any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(map[string]any{"allowedMcpServers": entries}, "", "  ")
	if err != nil {
		return err
	}
	return safefile.WriteFile(path, append(data, '\n'), 0o600)
}
