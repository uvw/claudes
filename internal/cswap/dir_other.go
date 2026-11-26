//go:build !linux

package cswap

import (
	"os"
	"path/filepath"
)

// Dir is cswap's backup directory: ~/.claude-swap-backup.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude-swap-backup"), nil
}
