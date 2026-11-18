package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// sameMeaning lists the letters that stand for different long flags on
// different commands, because they mean the same thing. -t is how many to
// show: the last events in logs, the biggest sessions in session du, and the
// best matches in session search.
var sameMeaning = map[string][]string{"t": {"tail", "top"}}

// safeguards are the flags that override a protection. They stay long, so
// that typing one is deliberate.
var safeguards = []string{"force", "purge", "pinned", "interactive", "interactive-bg", "rm", "forget", "no-redact"}

func TestEachShortFlagMeansOneThingEverywhere(t *testing.T) {
	letters := map[string]map[string][]string{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Shorthand == "" {
				return
			}
			if letters[f.Shorthand] == nil {
				letters[f.Shorthand] = map[string][]string{}
			}
			letters[f.Shorthand][f.Name] = append(letters[f.Shorthand][f.Name], cmd.CommandPath())
		})
		for _, c := range cmd.Commands() {
			walk(c)
		}
	}
	walk(newHarness(t).app.rootCommand())

	for letter, names := range letters {
		if len(names) == 1 {
			continue
		}
		allowed := map[string]bool{}
		for _, n := range sameMeaning[letter] {
			allowed[n] = true
		}
		for name, where := range names {
			if !allowed[name] {
				t.Errorf("-%s means --%s on %s, and something else elsewhere: %v", letter, name, strings.Join(where, ", "), names)
			}
		}
	}
	for _, names := range letters {
		for _, s := range safeguards {
			if where, ok := names[s]; ok {
				t.Errorf("--%s overrides a safeguard, so it has no short form, but has one on %s", s, strings.Join(where, ", "))
			}
		}
	}
	for letter, want := range map[string]string{"u": "outdated", "i": "in", "l": "since", "b": "bg", "r": "resume", "e": "regexp"} {
		if _, ok := letters[letter][want]; !ok {
			t.Errorf("-%s is not short for --%s: %v", letter, want, letters[letter])
		}
	}
}
