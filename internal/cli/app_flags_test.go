package cli

import (
	"strings"
	"testing"
	"time"

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

func TestDaysTakesCalendarWordsOnTheAppClock(t *testing.T) {
	h := newHarness(t)
	v := h.app.daysFlag(0)
	at := func(m time.Month, d, hour, minute int) time.Time {
		return time.Date(2026, m, d, hour, minute, 0, 0, time.UTC)
	}
	for _, c := range []struct {
		name string
		now  time.Time
		want map[string]time.Duration
	}{
		{"a Wednesday afternoon", at(time.November, 18, 15, 30), map[string]time.Duration{
			"today": 15*time.Hour + 30*time.Minute, "yesterday": 39*time.Hour + 30*time.Minute,
			"week": 63*time.Hour + 30*time.Minute, "month": 17*24*time.Hour + 15*time.Hour + 30*time.Minute,
		}},
		{"a Monday, where week is today", at(time.November, 16, 9, 0), map[string]time.Duration{
			"today": 9 * time.Hour, "yesterday": 33 * time.Hour, "week": 9 * time.Hour, "month": 15*24*time.Hour + 9*time.Hour,
		}},
		{"the 1st, where month is today", at(time.December, 1, 8, 0), map[string]time.Duration{
			"today": 8 * time.Hour, "yesterday": 32 * time.Hour, "week": 32 * time.Hour, "month": 8 * time.Hour,
		}},
		{"midnight, which still sets the flag", at(time.November, 16, 0, 0), map[string]time.Duration{
			"today": time.Nanosecond, "week": time.Nanosecond, "yesterday": 24 * time.Hour,
		}},
	} {
		h.now = c.now
		for word, want := range c.want {
			for _, typed := range []string{word, strings.ToUpper(word)} {
				if err := v.Set(typed); err != nil || v.d != want || v.String() != word {
					t.Errorf("%s: %s is %v and prints %q, %v; want %v", c.name, typed, v.d, v.String(), err, want)
				}
			}
		}
	}
	if err := v.Set("7d"); err != nil || v.d != 7*24*time.Hour || v.String() != "7d" {
		t.Errorf("7d after a word: %v %q %v", v.d, v.String(), err)
	}
	if err := v.Set("tomorrow"); err == nil || !strings.Contains(err.Error(), "today, yesterday, week, or month") {
		t.Errorf("tomorrow: %v", err)
	}
}

func TestEveryDurationFlagHasTheAppClock(t *testing.T) {
	var found []string
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if v, ok := f.Value.(*days); ok {
				found = append(found, cmd.CommandPath()+" --"+f.Name)
				if v.now == nil {
					t.Errorf("%s --%s has no clock: build it with App.daysFlag", cmd.CommandPath(), f.Name)
				}
			}
		})
		for _, c := range cmd.Commands() {
			walk(c)
		}
	}
	walk(newHarness(t).app.rootCommand())
	if len(found) < 6 {
		t.Errorf("found only %v", found)
	}
}

func TestCalendarStart(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no time zone data:", err)
	}
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, berlin) }
	for _, c := range []struct {
		name string
		now  time.Time
		want map[string]time.Time
	}{
		{"a Wednesday", at(time.November, 18, 15), map[string]time.Time{
			"today": at(time.November, 18, 0), "yesterday": at(time.November, 17, 0),
			"week": at(time.November, 16, 0), "month": at(time.November, 1, 0),
		}},
		{"a Monday", at(time.November, 16, 9), map[string]time.Time{
			"today": at(time.November, 16, 0), "yesterday": at(time.November, 15, 0),
			"week": at(time.November, 16, 0), "month": at(time.November, 1, 0),
		}},
		{"a Sunday", at(time.November, 22, 23), map[string]time.Time{
			"week": at(time.November, 16, 0),
		}},
		{"the 1st, a Tuesday", at(time.December, 1, 8), map[string]time.Time{
			"today": at(time.December, 1, 0), "yesterday": at(time.November, 30, 0),
			"week": at(time.November, 30, 0), "month": at(time.December, 1, 0),
		}},
		{"the 1st and a Monday", at(time.June, 1, 10), map[string]time.Time{
			"today": at(time.June, 1, 0), "week": at(time.June, 1, 0), "month": at(time.June, 1, 0),
		}},
		{"the 1st of the year", at(time.January, 1, 12), map[string]time.Time{
			"yesterday": time.Date(2025, time.December, 31, 0, 0, 0, 0, berlin),
			"week":      time.Date(2025, time.December, 29, 0, 0, 0, 0, berlin),
			"month":     at(time.January, 1, 0),
		}},
		{"the day the clocks go back", at(time.October, 25, 12), map[string]time.Time{
			"today": at(time.October, 25, 0), "yesterday": at(time.October, 24, 0),
		}},
	} {
		for word, want := range c.want {
			if got, ok := calendarStart(word, c.now); !ok || !got.Equal(want) {
				t.Errorf("%s: %s is %v %v, want %v", c.name, word, got, ok, want)
			}
		}
	}
}

func TestCalendarStartIgnoresCaseAndRefusesOtherWords(t *testing.T) {
	now := time.Date(2026, time.November, 18, 15, 0, 0, 0, time.UTC)
	for _, w := range []string{"TODAY", "Yesterday", "Week", "mOnTh"} {
		if _, ok := calendarStart(w, now); !ok {
			t.Errorf("%s refused", w)
		}
	}
	for _, w := range []string{"", "tomorrow", "weeks", "7d"} {
		if _, ok := calendarStart(w, now); ok {
			t.Errorf("%q accepted", w)
		}
	}
}
