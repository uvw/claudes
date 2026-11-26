// Package fake holds the files fakeclaude and the end-to-end tests share: the
// state the tests set up and read back, and the log of calls.
package fake

// Session is one entry of `claude agents --json`.
type Session struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name,omitempty"`
	Cwd       string `json:"cwd"`
	Kind      string `json:"kind"`
	State     string `json:"state,omitempty"`
	Status    string `json:"status,omitempty"`
	PID       int    `json:"pid"`
	StartedAt int64  `json:"startedAt"`
	// ConfigDir is the CLAUDE_CONFIG_DIR of the account the session runs
	// under, or "" for State.ConfigDir. fakeclaude acts on a session only
	// under its own, and leaves the key out of what it lists.
	ConfigDir string `json:"configDir,omitempty"`
}

// State is what the tests set up and read back.
type State struct {
	Version string `json:"version"`
	// AgentsExit is the exit code of a passthrough `claude agents` call.
	AgentsExit int `json:"agentsExit"`
	// Fail makes a subcommand print the message on stderr and exit 1.
	Fail     map[string]string `json:"fail,omitempty"`
	NextPID  int               `json:"nextPid"`
	Sessions []Session         `json:"sessions"`
	// ConfigDir is the CLAUDE_CONFIG_DIR of the account claudes runs as.
	ConfigDir string `json:"configDir"`
}

// Call is one invocation, as the tests read it from the log.
type Call struct {
	Args         []string `json:"args"`
	Cwd          string   `json:"cwd"`
	ClaudeCode   bool     `json:"claudeCode"`
	ClaudeJobDir bool     `json:"claudeJobDir"`
	// ConfigDir is the call's CLAUDE_CONFIG_DIR.
	ConfigDir string `json:"configDir"`
}
