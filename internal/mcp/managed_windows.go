//go:build windows

package mcp

import (
	"cmp"
	"os"
	"path/filepath"
)

// managedFiles are where Windows keeps managed-mcp.json.
func managedFiles() []string {
	return []string{filepath.Join(cmp.Or(os.Getenv("ProgramFiles"), `C:\Program Files`), "ClaudeCode", "managed-mcp.json")}
}
