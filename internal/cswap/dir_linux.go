//go:build linux

package cswap

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir is cswap's backup directory: $XDG_DATA_HOME/claude-swap when that is
// an absolute path, after a leading ~ is expanded, or else
// ~/.local/share/claude-swap.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		if xdg == "~" || strings.HasPrefix(xdg, "~/") {
			xdg = home + xdg[1:]
		}
		if filepath.IsAbs(xdg) {
			return filepath.Join(xdg, "claude-swap"), nil
		}
	}
	return filepath.Join(home, ".local", "share", "claude-swap"), nil
}
