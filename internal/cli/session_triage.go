package cli

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/transcripts"
)

// reviewToolCalls is the most tool calls a one-prompt session can make and
// still go to review. One that made more, such as a pull request review, did
// enough work to keep.
const reviewToolCalls = 20

// triageItem is one session or folder in the triage plan.
type triageItem struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"sessionId"`
	Name       string    `json:"name"`
	NameSource string    `json:"nameSource"`
	Cwd        string    `json:"cwd"`
	Size       int64     `json:"size"`
	LastActive time.Time `json:"lastActive"`
	// Why is the kind session gc removes it as, or why it is kept.
	Why        string                `json:"why"`
	LastPrompt string                `json:"lastPrompt"`
	Activity   *transcripts.Activity `json:"activity"`
}

type triageReport struct {
	// Remove is what session gc removes by default, and trivial sessions.
	Remove []triageItem `json:"remove"`
	// Review is the sessions only someone who reads their prompts can judge.
	Review []triageItem `json:"review"`
	Keep   []triageItem `json:"keep"`
}

func (app *App) sessionTriageCommand() *cobra.Command {
	var out outputFlag
	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Sort past sessions into remove, review, and keep",
		Long: `Read every past session and sort it into one of three groups. It changes
nothing.

  remove  what session gc takes without flags, and trivial sessions
  review  one prompt or only slash commands, no file changed, at most ` + strconv.Itoa(reviewToolCalls) + ` tool
          calls, and not named with /rename: whether to keep one takes reading
          its prompts
  keep    everything else, with the reason

-o json gives each review item's first and last prompt, so an agent can judge
them. Remove the sessions you pick with claudes session gc ID...`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := app.triage(cmd.Context())
			if err != nil {
				return err
			}
			if out.format != "text" {
				return out.encode(app.Out, report)
			}
			return app.printTriage(report)
		},
	}
	out.add(cmd, app.Config.Output.Format)
	return cmd
}

func (app *App) triage(ctx context.Context) (triageReport, error) {
	report := triageReport{Remove: []triageItem{}, Review: []triageItem{}, Keep: []triageItem{}}
	list, err := app.pastSessions(ctx)
	if err != nil {
		return report, err
	}
	if !app.runningKnown {
		return report, refused(errors.New("cannot tell which sessions are running, so session triage cannot sort them"))
	}
	keep, err := app.gcKeep(ctx, list)
	if err != nil {
		return report, err
	}
	items, err := app.gcPick(list, keep, gcFlags{empty: true, trivial: true, copies: true, leftovers: true})
	if err != nil {
		return report, err
	}
	byID := map[string]pastSession{}
	for _, p := range list {
		byID[p.SessionID] = p
	}
	removed := map[string]bool{}
	for _, it := range items {
		t := triageItem{ID: it.ID(), SessionID: it.sessionID, Name: it.name, Cwd: it.folder, Size: it.size, Why: it.why}
		if p, ok := byID[it.sessionID]; ok && (it.why == "empty" || it.why == "trivial") {
			removed[it.sessionID] = true
			readActivity(&p, false)
			t = triageOf(p, it.why)
			t.Size = it.size
		}
		report.Remove = append(report.Remove, t)
	}

	recent := app.Now().Add(-gcFloor)
	for _, p := range list {
		if removed[p.SessionID] {
			continue
		}
		if why, ok := keep[p.SessionID]; ok {
			report.Keep = append(report.Keep, triageOf(p, strings.TrimPrefix(why, "is ")))
			continue
		}
		if p.LastActive.After(recent) {
			report.Keep = append(report.Keep, triageOf(p, "changed in the last day"))
			continue
		}
		readActivity(&p, false)
		if why := keepWhy(p); why != "" {
			report.Keep = append(report.Keep, triageOf(p, why))
			continue
		}
		report.Review = append(report.Review, triageOf(p, "review"))
	}
	return report, nil
}

// keepWhy is why a past session that session gc would not take is kept, or ""
// when someone has to read it to tell.
func keepWhy(p pastSession) string {
	a := p.Activity
	switch {
	case a == nil:
		return "bigger than " + style.Size(transcripts.ActivityLimit)
	case p.NameSource == "custom":
		return "named with /rename"
	case a.Teammate:
		return "started by an agent team"
	case a.Edits > 0 || p.Disk.FileHistory > 0:
		return "changed files"
	case a.Prompts > 1:
		return "more than one prompt"
	case a.ToolCalls > reviewToolCalls:
		return "more than " + strconv.Itoa(reviewToolCalls) + " tool calls"
	case a.Prompts == 0 && a.Commands == 0:
		return "no prompt it could read"
	}
	return ""
}

func triageOf(p pastSession, why string) triageItem {
	return triageItem{ID: p.ShortID, SessionID: p.SessionID, Name: p.Name, NameSource: p.NameSource, Cwd: p.Cwd,
		Size: p.Disk.Total, LastActive: p.LastActive, Why: why, LastPrompt: p.LastPrompt, Activity: p.Activity}
}

func (app *App) printTriage(r triageReport) error {
	if len(r.Remove) > 0 {
		app.say("Remove %d session(s) and folder(s), %s:", len(r.Remove), style.Size(triageSize(r.Remove)))
		table := [][]string{{"ID", "WHY", "SIZE", "DIR", "NAME"}}
		for _, t := range r.Remove {
			table = append(table, []string{t.ID, t.Why, style.Size(t.Size), app.colors.Faint(homeShort(t.Cwd)), nameCell(cmp.Or(t.Name, t.ID))})
		}
		if err := style.Table(app.Out, table); err != nil {
			return err
		}
		app.say("\nTo remove them: %s\n", gcCommandFor(r.Remove))
	}
	if len(r.Review) > 0 {
		app.say("Review %d session(s), %s: one prompt or only commands, and no file changed:", len(r.Review), style.Size(triageSize(r.Review)))
		table := [][]string{{"ID", "TOOLS", "SIZE", "NAME", "FIRST PROMPT"}}
		for _, t := range r.Review {
			table = append(table, []string{t.ID, strconv.Itoa(t.Activity.ToolCalls), style.Size(t.Size), nameCell(cmp.Or(t.Name, t.ID)),
				shell.Printable(firstLine(cmp.Or(t.Activity.FirstPrompt, t.LastPrompt), 60))})
		}
		if err := style.Table(app.Out, table); err != nil {
			return err
		}
		app.say("\nRemove the ones you pick with: claudes session gc ID...\n")
	}
	counts := map[string]int{}
	for _, t := range r.Keep {
		counts[t.Why]++
	}
	reasons := make([]string, 0, len(counts))
	for why := range counts {
		reasons = append(reasons, why)
	}
	slices.SortFunc(reasons, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), cmp.Compare(a, b)) })
	app.say("Keep %d session(s):", len(r.Keep))
	for _, why := range reasons {
		app.say("  %4d  %s", counts[why], why)
	}
	return nil
}

func triageSize(items []triageItem) int64 {
	var n int64
	for _, t := range items {
		n += t.Size
	}
	return n
}

// gcCommandFor is the session gc command that removes exactly the kinds in
// items. Without trivial sessions, that is session gc's default.
func gcCommandFor(items []triageItem) string {
	kinds := map[string]bool{}
	for _, t := range items {
		kinds[t.Why] = true
	}
	cmd := "claudes session gc"
	if !kinds["trivial"] {
		return cmd
	}
	for _, k := range []struct{ why, flag string }{
		{"empty", "--empty"}, {"trivial", "--trivial"}, {"copy", "--copies"}, {"leftover", "--leftovers"},
	} {
		if kinds[k.why] {
			cmd += " " + k.flag
		}
	}
	return cmd
}
