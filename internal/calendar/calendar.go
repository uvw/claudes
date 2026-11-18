// Package calendar finds when a calendar word, such as today or week, began:
// the local midnight that starts it.
package calendar

import (
	"strings"
	"time"
)

// Words are the words Start takes.
var Words = []string{"today", "yesterday", "week", "month"}

// Start is the midnight, in now's location, that begins word: today,
// yesterday, the Monday of this week, or the 1st of this month. Letter case
// does not matter. ok is false for any other word.
func Start(word string, now time.Time) (start time.Time, ok bool) {
	y, m, d := now.Date()
	switch strings.ToLower(word) {
	case "today":
	case "yesterday":
		d--
	case "week":
		d -= (int(now.Weekday()) + 6) % 7
	case "month":
		d = 1
	default:
		return time.Time{}, false
	}
	return time.Date(y, m, d, 0, 0, 0, 0, now.Location()), true
}
