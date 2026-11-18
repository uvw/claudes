// Package install writes the claudes skill where agents load skills from.
package install

import (
	"path/filepath"

	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
)

const skillName = "claudes"

// ClaudeSkillsDir is Claude Code's personal skills directory: skills in Claude
// Code's directory (paths.ClaudeHome).
func ClaudeSkillsDir() (string, error) {
	home, err := paths.ClaudeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "skills"), nil
}

// Skill writes SKILL.md into dir/claudes and returns its path. It replaces an
// older copy, so running it again updates the skill.
func Skill(dir string, content []byte) (string, error) {
	path := filepath.Join(dir, skillName, "SKILL.md")
	return path, files.WriteFileAll(path, content, 0o644)
}
