package calendar

import (
	"testing"
	"time"
)

func TestStart(t *testing.T) {
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
			if got, ok := Start(word, c.now); !ok || !got.Equal(want) {
				t.Errorf("%s: %s is %v %v, want %v", c.name, word, got, ok, want)
			}
		}
	}
}

func TestStartIgnoresCaseAndRefusesOtherWords(t *testing.T) {
	now := time.Date(2026, time.November, 18, 15, 0, 0, 0, time.UTC)
	for _, w := range []string{"TODAY", "Yesterday", "Week", "mOnTh"} {
		if _, ok := Start(w, now); !ok {
			t.Errorf("%s refused", w)
		}
	}
	for _, w := range []string{"", "tomorrow", "weeks", "7d"} {
		if _, ok := Start(w, now); ok {
			t.Errorf("%q accepted", w)
		}
	}
}
