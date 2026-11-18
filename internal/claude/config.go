package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is ~/.claude.json, where Claude Code keeps the user's MCP servers,
// an entry for each project, and the feature flags it caches. claudes writes
// only a project's disabledMcpServers list in it, through SetDisabled.
type Config struct{ top map[string]json.RawMessage }

// ReadConfig reads ~/.claude.json from path. A missing file reads as empty. A
// relative path comes from a path left unset, and would read the current
// directory, so it reads as empty too.
func ReadConfig(path string) (Config, error) {
	if !filepath.IsAbs(path) {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return Config{top: m}, nil
}

// Key is the value of a top-level key, or nil.
func (c Config) Key(name string) json.RawMessage { return c.top[name] }

// Project is the entry ~/.claude.json keeps for dir.
func (c Config) Project(dir string) map[string]json.RawMessage {
	key, ok := c.EntryKey(dir)
	if !ok {
		return nil
	}
	return objectOf(objectOf(c.top["projects"])[key])
}

// EntryKey is the key ~/.claude.json keeps dir's entry under, and whether it
// has one. Claude Code keys a directory in a git repository by the
// repository's main worktree, and keys any other directory by itself, so two
// directories with one key share their settings.
func (c Config) EntryKey(dir string) (string, bool) {
	projects := objectOf(c.top["projects"])
	if len(projects) == 0 {
		return "", false
	}
	byDir := map[string]string{}
	for key := range projects {
		byDir[ProjectKey(key)] = key
	}
	// Claude Code follows links before it looks for the repository.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	for _, d := range []string{gitRoot(dir), dir} {
		if key, ok := byDir[ProjectKey(d)]; ok && d != "" {
			return key, true
		}
	}
	return "", false
}

// Disabled is the disabledMcpServers list of dir's entry: the servers /mcp
// turned off there. An item that is not a string is skipped.
func (c Config) Disabled(dir string) []string { return stringsOf(c.Project(dir)["disabledMcpServers"]) }

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

// ProjectKey is a directory as ~/.claude.json's projects name it, with forward
// slashes, which Claude Code writes on Windows too.
func ProjectKey(dir string) string { return filepath.ToSlash(filepath.Clean(dir)) }

// Feature reads a flag from the features Claude Code caches in the
// ~/.claude.json at path. found is false when the file or the flag is missing.
// Any value but false or null counts as on.
func Feature(path, flag string) (on, found bool, err error) {
	cfg, err := ReadConfig(path)
	if err != nil {
		return false, false, err
	}
	value, found := objectOf(cfg.top["cachedGrowthBookFeatures"])[flag]
	if !found {
		return false, false, nil
	}
	switch string(value) {
	case "false", "null":
		return false, true, nil
	}
	return true, true, nil
}

// objectOf decodes a JSON object, or nil when raw is not one.
func objectOf(raw json.RawMessage) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
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
