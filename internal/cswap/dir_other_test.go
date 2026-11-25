//go:build !linux

package cswap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsInTheHomeDirectory(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got, err := Dir(); err != nil || got != filepath.Join(home, ".claude-swap-backup") {
		t.Fatalf("dir %q, err %v", got, err)
	}
}
