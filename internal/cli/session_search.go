package cli

import (
	"cmp"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
)

// searchHit is a session search found, as -o json prints it.
type searchHit struct {
	ID         string              `json:"id"`
	SessionID  string              `json:"sessionId"`
	Name       string              `json:"name"`
	Cwd        string              `json:"cwd"`
	Score      float64             `json:"score"`
	LastActive time.Time           `json:"lastActive"`
	Matches    []transcripts.Match `json:"matches"`
}

type searchFlags struct {
	pattern, ran, touched, branch, in string
	who                               choiceFlag
	tools, noRedact                   bool
	since                             days
	top                               countFlag
}

func (app *App) sessionSearchCommand() *cobra.Command {
	f := searchFlags{who: choiceFlag{of: []string{"you", "claude"}}, top: countFlag{n: 20}, since: app.daysFlag(0)}
	var out render.Format
	cmd := &cobra.Command{
		Use:     "search QUERY...",
		Aliases: []string{"grep"},
		Short:   "Find past sessions by what was said in them",
		Long: `Find the sessions whose transcripts match every word of QUERY, best first. A
word matches in any case at the start of a word, so lock matches locks and
fileLock but not block, and quoted words match as a phrase. It looks in the
session's name, the prompts you typed, Claude's replies, and Claude's tool
calls: the commands it ran, and the paths and patterns it used. It skips slash
commands, what the tools returned, and the text Claude Code adds, such as
reminders and compaction summaries.

A session ranks by its best record: its name, or one prompt, reply, or tool
call. Each of those places weighs twice as much as the next. A record that holds
the words next to each other, in order, scores three times its place's weight,
one that holds every word twice, and one that holds some only their share. So a
prompt about a lock race outranks a session that says lock in one prompt and
race in another, and a prompt that holds both words outranks a reply that holds
the phrase. More matches and recent activity rank a session a little higher.

-e takes a regular expression in Go's RE2 syntax: Perl's, without lookaround or
backreferences. It matches anywhere, and is case-sensitive unless it starts with
(?i). --ran matches part of the commands of the Bash tool, and --touched part of
the paths of the files Claude edited or wrote. -e, --ran, and --touched each
work without QUERY.

Each line shows the best match, from the closest record, and -o json gives each
session's matches. Resume one with claudes session resume ID.

What it prints has secrets masked: provider keys such as sk-... and ghp_...,
Authorization credentials, the values of names that end in KEY, TOKEN, SECRET,
PASSWORD, or PASSWD, passwords in URLs, and private keys. So GITHUB_TOKEN=...
prints as GITHUB_TOKEN=[redacted]. QUERY still matches the text as written.
--no-redact prints it as is.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			hits, more, err := app.search(args, f)
			if err != nil {
				return err
			}
			if out != render.Text {
				return out.Encode(app.Out, hits)
			}
			if len(hits) == 0 {
				fmt.Fprintln(app.Out, "No sessions match.")
				return nil
			}
			table := [][]string{{"ID", "ACTIVE", "WHERE", "DIR", "NAME", "MATCH"}}
			for _, h := range hits {
				best := h.Matches[0]
				table = append(table, []string{h.ID, render.Age(app.Now(), h.LastActive), best.Where,
					render.DirCell(h.Cwd), render.NameCell(cmp.Or(h.Name, h.ID)), shell.Printable(best.Snippet)})
			}
			if err := app.writeTable(app.Out, table, tableFit{fitLast: true}); err != nil {
				return err
			}
			if more > 0 {
				app.say("\n%d more matched; --top 0 shows them all.", more)
			}
			app.say("\nResume the first with:\n  claudes session resume %s", hits[0].ID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&f.pattern, "regexp", "e", "", "a regular expression to match, in Go's RE2 syntax")
	cmd.Flags().Var(&f.who, "who", "match QUERY only in your prompts (you) or Claude's replies (claude)")
	cmd.RegisterFlagCompletionFunc("who", cobra.FixedCompletions(f.who.of, cobra.ShellCompDirectiveNoFileComp))
	cmd.Flags().BoolVar(&f.tools, "tools", false, "also search what the tools returned")
	cmd.MarkFlagsMutuallyExclusive("who", "tools")
	cmd.Flags().StringVar(&f.ran, "ran", "", "only sessions where Claude ran a shell command containing this")
	cmd.Flags().StringVar(&f.touched, "touched", "", "only sessions where Claude edited or wrote a file whose path contains this")
	cmd.Flags().StringVar(&f.branch, "branch", "", "only sessions on this git branch")
	cmd.Flags().StringVarP(&f.in, "in", "i", "", "only sessions run in this directory or below it")
	cmd.Flags().VarP(&f.since, "since", "l", "only sessions active within this long, such as 7d, 12h, or today")
	cmd.Flags().Lookup("since").DefValue = ""
	cmd.Flags().VarP(&f.top, "top", "t", "show the N best sessions; 0 shows all")
	cmd.Flags().BoolVar(&f.noRedact, "no-redact", false, "print secrets instead of masking them")
	app.addOutput(cmd, &out, app.Config.Output.Format)
	return cmd
}

// search returns the best sessions that match, and how many more matched
// beyond --top.
func (app *App) search(words []string, f searchFlags) ([]searchHit, int, error) {
	switch {
	case len(words) == 0 && f.pattern == "" && f.ran == "" && f.touched == "":
		return nil, 0, errors.New("name something to search for: QUERY, -e PATTERN, --ran, or --touched")
	}
	list, err := app.Transcripts.List()
	if err != nil {
		return nil, 0, err
	}
	past := make([]pastSession, len(list))
	for i, s := range list {
		past[i] = pastSession{ShortID: s.ID(), Session: s}
	}
	if past, err = app.filterPast(past, nil, f.in, f.since.d); err != nil {
		return nil, 0, err
	}
	var sessions []transcripts.Session
	for _, p := range past {
		if f.branch == "" || p.Branch == f.branch {
			sessions = append(sessions, p.Session)
		}
	}
	results, err := transcripts.Search(sessions, transcripts.Query{
		Words: words, Pattern: f.pattern, Who: f.who.value, Tools: f.tools, Ran: f.ran, Touched: f.touched, Now: app.Now(),
		Raw: f.noRedact,
	})
	if err != nil {
		if f.pattern != "" {
			return nil, 0, fmt.Errorf("-e: %w", err)
		}
		return nil, 0, err
	}
	more := 0
	if f.top.n > 0 && len(results) > f.top.n {
		results, more = results[:f.top.n], len(results)-f.top.n
	}
	hits := make([]searchHit, len(results))
	for i, r := range results {
		name := r.Name
		if !f.noRedact {
			name = transcripts.Redact(name)
		}
		hits[i] = searchHit{ID: r.ID(), SessionID: r.SessionID, Name: name, Cwd: r.Cwd, Score: r.Score, LastActive: r.LastActive, Matches: r.Matches}
	}
	return hits, more, nil
}
