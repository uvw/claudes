package cli

import (
	"testing"
	"time"
)

func TestAgeIsShortAndCoarse(t *testing.T) {
	for ago, want := range map[time.Duration]string{
		10 * time.Second: "now",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		50 * time.Hour:   "2d",
	} {
		if got := age(start, start.Add(-ago)); got != want {
			t.Errorf("%s ago: got %q, want %q", ago, got, want)
		}
	}
	if got := age(start, time.Time{}); got != "-" {
		t.Errorf("no start time: got %q", got)
	}
}
