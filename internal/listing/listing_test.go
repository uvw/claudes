package listing

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

// Scripts read claudes ls -o json, so a key never changes its name.
func TestRowJSONKeepsItsKeys(t *testing.T) {
	data, err := json.Marshal(Row{})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"id", "sessionId", "name", "cwd", "kind", "state", "status", "pid", "startedAt",
		"pinned", "version", "outdated", "teammates", "subagents", "shells", "tokens", "prs", "doing",
	}
	var keys []string
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		t.Fatalf("keys %q, want %q", keys, want)
	}
}

func TestSortPutsPinnedThenBusyThenNewestFirst(t *testing.T) {
	row := func(name string, pinned bool, status string, started int64) Row {
		return Row{Session: claude.Session{Name: name, Status: status, PID: 1, StartedAt: started}, Pinned: pinned}
	}
	rows := []Row{
		row("old", false, "idle", 1),
		row("new", false, "idle", 3),
		row("busy", false, "busy", 2),
		row("pinned", true, "idle", 0),
	}
	Sort(rows)
	var got []string
	for _, r := range rows {
		got = append(got, r.Name)
	}
	if want := []string{"pinned", "busy", "new", "old"}; !slices.Equal(got, want) {
		t.Fatalf("order %q, want %q", got, want)
	}
}

func TestDoingAndZeroIfKnown(t *testing.T) {
	text := "fix\x1b[31m it"
	if Doing(Row{}) != "-" || Doing(Row{Doing: &text}) != "fix[31m it" {
		t.Errorf("doing %q", Doing(Row{Doing: &text}))
	}
	n := 3
	if ZeroIfKnown(nil) != nil || *ZeroIfKnown(&n) != 0 {
		t.Error("ZeroIfKnown")
	}
}
