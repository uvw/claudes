// Package cli is the claudes command line.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/version"
)

// Main runs claudes and returns its exit code.
func Main(args []string) int { return newApp().Run(args) }

// fixesConfig reports whether command still runs with a broken config.yaml:
// config and doctor help repair it, and the rest read nothing from it. A
// shell profile runs init and completion, so failing them would put the error
// in every new shell.
func fixesConfig(command string) bool {
	switch command {
	case "config", "doctor", "help", "-h", "--help", "--version",
		"init", "completion", "skill", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return true
	}
	return false
}

// Run runs claudes with app's dependencies and returns the exit code.
func (app *App) Run(args []string) int {
	var env []string
	if app.Environ != nil {
		env = app.Environ()
	}
	app.colors, app.errColors = style.Detect(app.Out, env), style.Detect(app.Err, env)
	root := app.rootCommand()
	if passThrough(root, args) {
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if err := app.Exec("claude", append([]string{"agents"}, args...)...); err != nil {
			app.errorLine("claudes: cannot run claude agents: %v", err)
			return 1
		}
		return 0
	}
	if app.setupErr != nil {
		app.errorLine("claudes: %v", app.setupErr)
		return 1
	}
	if app.configErr != nil && !fixesConfig(commandName(args)) {
		app.errorLine("claudes: %v", app.configErr)
		fmt.Fprintln(app.Err, "Fix it with: claudes config edit")
		return 1
	}
	if name := os.Getenv("CLAUDES_LOG"); name != "" {
		if err := app.setLogLevel(name); err != nil {
			app.warn("claudes: %v", err)
		}
	}
	ctx, stop := app.Interrupt(context.Background())
	defer stop()
	// After the first Ctrl-C, commands finish the session in hand, which runs
	// under context.WithoutCancel, and report. A second one ends claudes at
	// once.
	go func() {
		<-ctx.Done()
		stop()
	}()
	root.SetArgs(args)
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	if args[0] == cobra.ShellCompRequestCmd || args[0] == cobra.ShellCompNoDescRequestCmd {
		// Cobra notes each completion's directive on stderr, and not every
		// shell's completion script hides it.
		root.SetErr(io.Discard)
	}
	defer app.unlock()
	defer app.printReport()
	err := root.ExecuteContext(ctx)
	app.record(func(r *report) {
		switch {
		case ctx.Err() != nil && !app.endsOnInterrupt:
			r.Error = "interrupted"
		case err != nil && !errors.Is(err, exit.ErrSomeFailed) && !errors.Is(err, exit.ErrReported):
			r.Error = err.Error()
		}
	})
	switch {
	case ctx.Err() != nil && app.endsOnInterrupt && err == nil:
		return 0
	case ctx.Err() != nil:
		fmt.Fprintln(app.Err)
		app.errorLine("claudes: interrupted. Some sessions may already be stopped or restarted: check claudes ls")
		return exit.Interrupted
	case err == nil:
		return 0
	case errors.Is(err, exit.ErrSomeFailed):
		return exit.SomeFailed
	case errors.Is(err, exit.ErrReported):
		return exit.Failed
	}
	app.errorLine("claudes: %s", shell.PrintableText(err.Error()))
	return exit.Code(err)
}

// commandName is the first argument after claudes' own -v or --verbose, or
// "" when there is none.
func commandName(args []string) string {
	for _, a := range args {
		if a != "-v" && a != "--verbose" && !strings.HasPrefix(a, "--verbose=") {
			return a
		}
	}
	return ""
}

// passThrough reports whether the arguments belong to claude agents: no
// arguments, a first argument of --, or a first argument that is not one of
// claudes' own. The -- form keeps working when claudes adds a command that
// claude agents also has.
func passThrough(root *cobra.Command, args []string) bool {
	if len(args) == 0 || args[0] == "--" {
		return true
	}
	first := commandName(args)
	switch first {
	case "", "-h", "--help", "help", "--version", "completion",
		cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return false
	}
	for _, c := range root.Commands() {
		if c.Name() == first || slices.Contains(c.Aliases, first) {
			return false
		}
	}
	return true
}

func (app *App) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "claudes",
		Short: "A wrapper around claude agents",
		Long: `claudes lists, pins, restarts, pauses, and resumes Claude Code sessions.

Run it with no command, or with arguments that are not a claudes command, and it
runs claude agents with those arguments. claudes -- ARGS always does, even when
ARGS starts with a claudes command.

Bulk commands leave pinned sessions alone unless you name them. Pins are the
same as Ctrl+T in agent view.`,
		Version:       version.Build(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
				if err := app.setLogLevel("debug"); err != nil {
					return err
				}
			}
			app.log().Debug("config", "path", app.ConfigPath)
			app.startReport(cmd)
			return app.lock(cmd)
		},
	}
	root.PersistentFlags().BoolP("verbose", "v", false, "log each claude call and step to stderr")
	root.AddCommand(
		app.lsCommand(),
		alone(reports(app.pinCommand(true))),
		alone(reports(app.pinCommand(false))),
		alone(reports(app.restartCommand())),
		alone(reports(app.pauseCommand())),
		alone(reports(app.resumeCommand())),
		alone(reports(app.pingCommand())),
		alone(reports(app.stopCommand())),
		app.logsCommand(),
		app.attachCommand(),
		app.whereCommand(),
		alone(reports(app.gcCommand())),
		app.skillCommand(),
		app.configCommand(),
		app.doctorCommand(),
		app.sessionCommand(),
		app.teamCommand(),
		app.mcpCommand(),
		app.cdCommand(),
		app.initCommand(),
		app.upgradeCommand(),
	)
	// pwsh names PowerShell here, as in claudes init pwsh. The completion
	// scripts write to the output set when they are made, so set it first.
	root.SetOut(app.Out)
	root.SetErr(app.Err)
	root.InitDefaultCompletionCmd()
	if ps, _, err := root.Find([]string{"completion", "powershell"}); err == nil && ps.Name() == "powershell" {
		ps.Aliases = append(ps.Aliases, "pwsh")
	}
	// Flag help wraps to fit 80 columns, as the rest of the help does.
	root.SetUsageTemplate(strings.NewReplacer(
		".LocalFlags.FlagUsages", ".LocalFlags.FlagUsagesWrapped 80",
		".InheritedFlags.FlagUsages", ".InheritedFlags.FlagUsagesWrapped 80",
	).Replace(root.UsageTemplate()))
	return root
}
