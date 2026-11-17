// Package paths says where claudes keeps its own files: its config, its state
// such as the park file, and its cache. It follows the XDG base directory
// variables on every platform, and each platform's own directories otherwise. It
// also finds Claude Code's own directory.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

const app = "claudes"

// Config is the directory holding config.yaml: $XDG_CONFIG_HOME/claudes, or
// ~/.config/claudes on macOS and Linux, or %APPDATA%\claudes on Windows.
func Config() (string, error) { return dir("XDG_CONFIG_HOME", configDefault) }

// State is the directory for files claudes keeps between runs, such as the park
// file: $XDG_STATE_HOME/claudes, ~/.local/state/claudes, or
// %LOCALAPPDATA%\claudes\state on Windows.
func State() (string, error) { return dir("XDG_STATE_HOME", stateDefault) }

// Cache is the directory for files claudes can rebuild: $XDG_CACHE_HOME/claudes,
// ~/.cache/claudes, or %LOCALAPPDATA%\claudes\cache on Windows.
func Cache() (string, error) { return dir("XDG_CACHE_HOME", cacheDefault) }

// dir uses the XDG variable when it holds an absolute path, as the XDG spec
// says, and the platform default otherwise.
func dir(variable string, platformDefault func() (string, error)) (string, error) {
	if base := os.Getenv(variable); filepath.IsAbs(base) {
		return filepath.Join(base, app), nil
	}
	d, err := platformDefault()
	if err != nil {
		return "", fmt.Errorf("cannot find a directory for claudes' files: %w", err)
	}
	return d, nil
}

// ClaudeHome is Claude Code's directory: $CLAUDE_CONFIG_DIR, or ~/.claude. It
// fails rather than guess: a wrong directory has no pins, and bulk commands would
// then act on pinned sessions.
func ClaudeHome() (string, error) {
	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		h, err := home(".claude")
		if err != nil {
			return "", fmt.Errorf("cannot find the Claude Code directory: %w", err)
		}
		return h, nil
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("CLAUDE_CONFIG_DIR must be an absolute path, not %q", base)
	}
	return base, nil
}

func home(parts ...string) (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{h}, parts...)...), nil
}
