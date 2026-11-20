package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/selection"
)

// sessionKind picks the sessions a command can act on, so completion offers
// only those.
type sessionKind int

const (
	anySession sessionKind = iota
	runningSession
	backgroundSession
	pinnedSession
	unpinnedSession
	finishedSession
)

func (app *App) keeps(k sessionKind, s claude.Session) bool {
	background := !s.Interactive() && s.JobID != ""
	switch k {
	case anySession:
		return true
	case runningSession:
		return s.Running()
	case backgroundSession:
		return background
	case pinnedSession, unpinnedSession:
		if !background {
			return false
		}
		pins, err := app.Jobs.PinSet()
		return err == nil && pins[s.JobID] == (k == pinnedSession)
	case finishedSession:
		return selection.IsFinished(s)
	}
	return true
}

// completeSessions completes the names of the sessions of kind k. With
// single, it stops after the first name, for commands that take one target.
func (app *App) completeSessions(k sessionKind, single bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if single && len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return app.sessionCompletions(cmd, k, args), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeSkip completes --skip with the sessions of kind k.
func (app *App) completeSkip(k sessionKind) cobra.CompletionFunc {
	return func(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		skip, _ := cmd.Flags().GetStringArray("skip")
		return app.sessionCompletions(cmd, k, skip), cobra.ShellCompDirectiveNoFileComp
	}
}

func (app *App) sessionCompletions(cmd *cobra.Command, k sessionKind, taken []string) []cobra.Completion {
	all := k != runningSession
	sessions, err := app.Agents.Sessions(cmd.Context(), all)
	if err != nil {
		return nil
	}
	var out []cobra.Completion
	for _, s := range sessions {
		if !app.keeps(k, s) || slices.Contains(taken, s.Label()) {
			continue
		}
		out = append(out, cobra.CompletionWithDesc(s.Label(), app.describeForCompletion(s)))
	}
	return out
}

// completeResumable completes the sessions resume can bring back: parked ones
// first, then stopped ones.
func (app *App) completeResumable(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var out []cobra.Completion
	parked := map[string]bool{}
	if entries, err := app.Park.Load(); err == nil {
		for _, e := range entries {
			parked[e.SessionID] = true
			if !slices.Contains(args, e.Label()) {
				out = append(out, cobra.CompletionWithDesc(e.Label(), e.ShortID()+" parked"))
			}
		}
	}
	forget, _ := cmd.Flags().GetBool("forget")
	if sessions, err := app.Agents.Sessions(cmd.Context(), true); err == nil && !forget {
		for _, s := range sessions {
			if s.Running() || parked[s.SessionID] || slices.Contains(args, s.Label()) {
				continue
			}
			out = append(out, cobra.CompletionWithDesc(s.Label(), app.describeForCompletion(s)))
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func (app *App) completeResumeSkip(cmd *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	skip, _ := cmd.Flags().GetStringArray("skip")
	var out []cobra.Completion
	if entries, err := app.Park.Load(); err == nil {
		for _, e := range entries {
			if !slices.Contains(skip, e.Label()) {
				out = append(out, cobra.CompletionWithDesc(e.Label(), e.ShortID()+" parked"))
			}
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func (app *App) describeForCompletion(s claude.Session) string {
	return fmt.Sprintf("%s %s %s", s.ID(), s.Mode(), render.Dash(s.State))
}

func (app *App) completeKeys(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]cobra.Completion, 0, len(config.Keys))
	for _, k := range config.Keys {
		out = append(out, cobra.CompletionWithDesc(k.Name, k.About))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func (app *App) completePast(single bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if single && len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		list, err := app.pastSessions(cmd.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var out []cobra.Completion
		for _, p := range list {
			state := "-"
			if p.Running {
				state = "running"
			}
			out = append(out, cobra.CompletionWithDesc(p.Label(),
				fmt.Sprintf("%s %s %s", p.ShortID, state, render.HomeShort(p.Cwd))))
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}
