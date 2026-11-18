//go:build unix

package mcp

// managedFiles are where macOS and Linux keep managed-mcp.json.
func managedFiles() []string {
	return []string{
		"/Library/Application Support/ClaudeCode/managed-mcp.json",
		"/etc/claude-code/managed-mcp.json",
	}
}
