package cli

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
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

// days is a duration flag that also accepts a number of days, such as 7d.
type days struct{ d time.Duration }

func (v *days) String() string {
	if v.d > 0 && v.d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(v.d/(24*time.Hour))) + "d"
	}
	return v.d.String()
}

func (v *days) Set(s string) error {
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
		return errors.New("use a duration such as 7d or 12h")
	}
	v.d = d
	return nil
}

func (v *days) Type() string { return "duration" }
