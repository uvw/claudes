package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
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
		return isFinished(s)
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
		out = append(out, cobra.CompletionWithDesc(s.Label(), describeForCompletion(s)))
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
			out = append(out, cobra.CompletionWithDesc(s.Label(), describeForCompletion(s)))
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

func describeForCompletion(s claude.Session) string {
	return fmt.Sprintf("%s %s %s", s.ID(), kind(s), dash(s.State))
}
