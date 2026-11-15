// Package skills carries the agent skills in this folder, so the claudes
// binary can print and install them. The same files serve `npx skills add`,
// which reads skills/<name>/SKILL.md from the repository.
package skills

import _ "embed"

// Claudes is the skill that teaches an agent to use claudes.
//
//go:embed claudes/SKILL.md
var Claudes []byte
