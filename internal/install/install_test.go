package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeSkillsDirIsUnderClaudeHome(t *testing.T) {
	abs := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", abs)
	if dir, err := ClaudeSkillsDir(); err != nil || dir != filepath.Join(abs, "skills") {
		t.Fatalf("dir %q, err %v", dir, err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "relative")
	if dir, err := ClaudeSkillsDir(); err == nil {
		t.Fatalf("accepted a relative CLAUDE_CONFIG_DIR: %q", dir)
	}
}

func TestSkillWritesAndReplacesSkillMD(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	for _, content := range []string{"first", "second"} {
		path, err := Skill(dir, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		if path != filepath.Join(dir, "claudes", "SKILL.md") {
			t.Fatalf("path %q", path)
		}
		if got, _ := os.ReadFile(path); string(got) != content {
			t.Fatalf("SKILL.md %q, want %q", got, content)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "claudes")); len(entries) != 1 {
		t.Fatalf("files left beside SKILL.md: %v", entries)
	}
}
