//go:build unix && !darwin

package mcp

// managedFiles are where Linux, and Claude Code on any other Unix, keep
// managed-mcp.json.
func managedFiles() []string {
	return []string{"/etc/claude-code/managed-mcp.json"}
}
