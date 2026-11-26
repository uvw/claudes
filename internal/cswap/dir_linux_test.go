//go:build linux

package cswap

import (
	"path/filepath"
	"testing"
)

func TestDirFollowsXDGDataHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for xdg, want := range map[string]string{
		"":          filepath.Join(home, ".local", "share", "claude-swap"),
		"relative":  filepath.Join(home, ".local", "share", "claude-swap"),
		"/data":     "/data/claude-swap",
		"~/data":    filepath.Join(home, "data", "claude-swap"),
		"/data/x//": "/data/x/claude-swap",
	} {
		t.Setenv("XDG_DATA_HOME", xdg)
		if got, err := Dir(); err != nil || got != want {
			t.Errorf("XDG_DATA_HOME=%q: %q, %v, want %q", xdg, got, err, want)
		}
	}
}
