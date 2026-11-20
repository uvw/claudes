//go:build darwin

package mcp

// managedFiles are where macOS keeps managed-mcp.json.
func managedFiles() []string {
	return []string{"/Library/Application Support/ClaudeCode/managed-mcp.json"}
}
