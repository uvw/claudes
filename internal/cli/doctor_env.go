package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/compat"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/version"
)

// envVars are the variables that change where claudes looks or how it runs.
var envVars = []string{
	"CLAUDES_CONFIG", "CLAUDES_LOG", "CLAUDE_CONFIG_DIR",
	"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME",
	"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA",
	"VISUAL", "EDITOR", "NO_COLOR", "CLICOLOR_FORCE", "FORCE_COLOR", "CLICOLOR",
	"CLAUDE_CODE_SESSION_ID", "CLAUDECODE", "CLAUDE_JOB_DIR",
}

// envSection is one group of claudes doctor env, in the order it prints.
type envSection struct {
	name  string
	pairs [][2]string
}

func (app *App) doctorEnvCommand() *cobra.Command {
	var out render.Format
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print versions, claudes' files, and the variables that move them",
		Long: `Print claudes' and claude's versions, where claudes finds its own files and
Claude Code's, and the environment variables that change either. Paste it into
a bug report.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sections := app.environment(cmd.Context())
			if out != render.Text {
				report := map[string]map[string]string{}
				for _, s := range sections {
					report[s.name] = map[string]string{}
					for _, kv := range s.pairs {
						report[s.name][kv[0]] = kv[1]
					}
				}
				return out.Encode(app.Out, report)
			}
			tw := tabwriter.NewWriter(app.Out, 0, 0, 2, ' ', 0)
			for i, s := range sections {
				if i > 0 {
					fmt.Fprintln(tw)
				}
				fmt.Fprintf(tw, "%s\n", s.name)
				for _, kv := range s.pairs {
					fmt.Fprintf(tw, "  %s\t%s\n", kv[0], render.Dash(kv[1]))
				}
			}
			return tw.Flush()
		},
	}
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

func (app *App) environment(ctx context.Context) []envSection {
	exe, _ := app.Executable()
	claudes := envSection{"claudes", [][2]string{
		{"version", version.Build()},
		{"executable", exe},
		{"platform", runtime.GOOS + "/" + runtime.GOARCH},
		{"go", runtime.Version()},
	}}

	installed, err := app.Agents.Version(ctx)
	if err != nil {
		installed = "error: " + err.Error()
	}
	claudeCode := envSection{"claude", [][2]string{
		{"path", app.claudePath()},
		{"version", installed},
		{"tested", compat.Tested.String()},
		{"this session", app.HostID},
	}}

	configFile := app.ConfigPath
	switch {
	case app.configErr != nil:
		configFile += " (" + app.configErr.Error() + ")"
	case configFile != "" && !config.Exists(configFile):
		configFile += " (none, so the built-in defaults apply)"
	}
	cache := "(no cache directory, so claude --version runs every time)"
	if app.CacheDir != "" {
		cache = compat.CachePath(app.CacheDir)
	}
	files := envSection{"files", [][2]string{
		{"config", configFile},
		{"park file", app.Park.Path},
		{"version cache", cache},
		{"claude home", filepath.Dir(app.Jobs.Dir)},
		{"jobs", app.Jobs.Dir},
		{"transcripts", app.Transcripts.Dir},
	}}

	var vars []string
	if app.Environ != nil {
		vars = app.Environ()
	}
	env := envSection{name: "environment"}
	for _, name := range envVars {
		// The last value wins, as it does for the colors.
		var value string
		var set bool
		for _, kv := range vars {
			if k, v, ok := strings.Cut(kv, "="); ok && k == name {
				value, set = v, true
			}
		}
		if set && value == "" {
			value = `""`
		}
		env.pairs = append(env.pairs, [2]string{name, value})
	}
	return []envSection{claudes, claudeCode, files, env}
}
