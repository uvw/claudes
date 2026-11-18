package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/skills"
)

func (app *App) skillCommand() *cobra.Command {
	var doInstall bool
	var dir string
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the agent skill for claudes, or install it for Claude Code",
		Long: `Print the agent skill that teaches a coding agent to use claudes. --install
writes it to ~/.claude/skills/claudes/SKILL.md, or under $CLAUDE_CONFIG_DIR, so
Claude Code loads it. --dir writes it under another skills directory instead.
Running it again updates the skill to this version of claudes.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if !doInstall && dir == "" {
				_, err := app.Out.Write(skills.Claudes)
				return err
			}
			target := dir
			if target == "" {
				var err error
				if target, err = claudeSkillsDir(); err != nil {
					return err
				}
			}
			path, err := installSkill(target, skills.Claudes)
			if err != nil {
				return err
			}
			app.say("Installed the claudes skill at %s", path)
			return nil
		},
	}
	cmd.Flags().BoolVar(&doInstall, "install", false, "install the skill for Claude Code")
	cmd.Flags().StringVar(&dir, "dir", "", "install the skill under this skills directory instead")
	return cmd
}

// claudeSkillsDir is Claude Code's personal skills directory, under
// paths.ClaudeHome.
func claudeSkillsDir() (string, error) {
	home, err := paths.ClaudeHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "skills"), nil
}

// installSkill writes SKILL.md into dir/claudes and returns its path. It
// replaces an older copy, so running it again updates the skill.
func installSkill(dir string, content []byte) (string, error) {
	path := filepath.Join(dir, "claudes", "SKILL.md")
	return path, files.WriteFileAll(path, content, 0o644)
}
