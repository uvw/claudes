package cli

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/calendar"
)

// lifecycleFlags are the flags of restart, pause, and resume.
type lifecycleFlags struct {
	planFlags
	skip          []string
	wait          bool
	timeout       time.Duration
	ping          bool
	pingText      string
	interactive   bool
	interactiveBG bool
	outdated      bool
	forget        bool
	// pinned has bulk commands take pinned sessions too.
	pinned bool
	// pingDefault is what --ping sends: the config's ping.text, or the
	// built-in prompt.
	pingDefault string
}

func (f *lifecycleFlags) addCommon(cmd *cobra.Command, completeSkip cobra.CompletionFunc) {
	f.planFlags.add(cmd)
	cmd.Flags().StringArrayVarP(&f.skip, "skip", "s", nil, "leave out a session, matched like a target (repeatable)")
	cmd.RegisterFlagCompletionFunc("skip", completeSkip)
}

func (f *lifecycleFlags) addWait(cmd *cobra.Command, timeout time.Duration) {
	cmd.Flags().BoolVarP(&f.wait, "wait", "w", false, "wait for busy sessions to go idle first")
	cmd.Flags().DurationVar(&f.timeout, "timeout", timeout, "with --wait, give up after this long")
}

func (f *lifecycleFlags) addPing(cmd *cobra.Command, prompt string) {
	f.pingDefault = prompt
	cmd.Flags().BoolVarP(&f.ping, "ping", "p", false, "send a prompt to sessions that were mid-turn, so they carry on")
	cmd.Flags().StringVar(&f.pingText, "ping-text", "", "the prompt to send (implies --ping)")
}

// addInteractive adds the flags that let restart and pause act on interactive
// sessions. The two modes cannot be combined.
func (f *lifecycleFlags) addInteractive(cmd *cobra.Command, stopUsage, bgUsage string) {
	cmd.Flags().BoolVar(&f.interactive, "interactive", false, stopUsage)
	if bgUsage == "" {
		return
	}
	cmd.Flags().BoolVar(&f.interactiveBG, "interactive-bg", false, bgUsage)
	cmd.MarkFlagsMutuallyExclusive("interactive", "interactive-bg")
}

// addPinned adds --pinned, which lets a bulk restart or pause take pinned
// sessions too.
func (f *lifecycleFlags) addPinned(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVar(&f.pinned, "pinned", false, usage)
}

// text is the ping prompt, or "" when nothing is pinged.
func (f *lifecycleFlags) text() string {
	switch {
	case f.pingText != "":
		return f.pingText
	case f.ping:
		return f.pingDefault
	}
	return ""
}

// planFlags are --dry-run and --confirm, the safeguards of every command that
// prints its plan before it acts.
type planFlags struct{ dryRun, confirm bool }

func (f *planFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&f.dryRun, "dry-run", "n", false, "show the plan and change nothing")
	cmd.Flags().BoolVarP(&f.confirm, "confirm", "c", false, "ask y/N before changing anything")
}

// forceUsage is --force's help on the commands that act on sessions.
const forceUsage = "go ahead even when Claude Code's output looks unfamiliar (see claudes doctor)"

// addForce adds --force to a command that changes sessions or pins.
func (app *App) addForce(cmd *cobra.Command, usage string) {
	cmd.Flags().BoolVar(&app.force, "force", false, usage)
}

// days is a duration flag that also accepts a number of days, such as 7d, or a
// calendar word, such as today, which stores the span from that word's start
// to now. now reads App.Now when the flag is parsed, so the span and the times
// it is measured against come from the same clock. Build one with
// App.daysFlag, which sets now.
type days struct {
	d    time.Duration
	word string
	now  func() time.Time
}

// daysFlag is a days flag that defaults to d.
func (app *App) daysFlag(d time.Duration) days {
	return days{d: d, now: func() time.Time { return app.Now() }}
}

func (v *days) String() string {
	switch {
	case v.word != "":
		return v.word
	case v.d > 0 && v.d%(24*time.Hour) == 0:
		return strconv.Itoa(int(v.d/(24*time.Hour))) + "d"
	}
	return v.d.String()
}

func (v *days) Set(s string) error {
	v.word = ""
	if word := strings.ToLower(s); slices.Contains(calendar.Words, word) {
		now := v.now()
		start, _ := calendar.Start(word, now)
		// 0 means the flag is unset, so a word given at the very midnight it
		// starts at still takes a span.
		v.d, v.word = max(now.Sub(start), time.Nanosecond), word
		return nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		count, err := strconv.Atoi(n)
		if err != nil || count < 0 {
			return errors.New("use a whole number of days, such as 7d")
		}
		v.d = time.Duration(count) * 24 * time.Hour
		return nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return errors.New("use a duration such as 7d or 12h, or today, yesterday, week, or month")
	}
	v.d = d
	return nil
}

func (v *days) Type() string { return "duration" }

// countFlag is a flag that takes a whole number, 0 or more.
type countFlag struct{ n int }

func (c *countFlag) String() string { return strconv.Itoa(c.n) }

func (c *countFlag) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fmt.Errorf("must be a whole number, 0 or more, not %q", s)
	}
	c.n = n
	return nil
}

func (c *countFlag) Type() string { return "int" }

// choiceFlag is a flag that takes one of a few words, so a typo fails when the
// flags are parsed.
type choiceFlag struct {
	value string
	of    []string
}

func (c *choiceFlag) String() string { return c.value }

func (c *choiceFlag) Set(s string) error {
	if !slices.Contains(c.of, s) {
		return fmt.Errorf("must be %s, not %q", orList(c.of), s)
	}
	c.value = s
	return nil
}

func (c *choiceFlag) Type() string { return "string" }

// orList joins words as "a or b", or "a, b, or c".
func orList(words []string) string {
	if len(words) < 3 {
		return strings.Join(words, " or ")
	}
	return strings.Join(words[:len(words)-1], ", ") + ", or " + words[len(words)-1]
}

// waitTimeout is the default of --timeout: wait.timeout from the config, or 30
// minutes.
func (app *App) waitTimeout() time.Duration {
	if d := time.Duration(app.Config.Wait.Timeout); d > 0 {
		return d
	}
	return 30 * time.Minute
}

// pingPrompt is ping.text from the config, or builtin.
func (app *App) pingPrompt(builtin string) string {
	if app.Config.Ping.Text != "" {
		return app.Config.Ping.Text
	}
	return builtin
}
