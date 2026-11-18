package render

import (
	"encoding/json"
	"slices"
	"testing"
)

// Scripts read the report's keys, so renaming one breaks them.
func TestReportKeepsItsFieldNames(t *testing.T) {
	r := NewReport("session gc", true)
	r.Plan = append(r.Plan, Entry{ID: "i", Name: "n", Cwd: "c", Result: "r", Note: "o", Command: "m", Error: "e"})
	got, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"command":"session gc","dryRun":true,` +
		`"plan":[{"id":"i","name":"n","cwd":"c","result":"r","note":"o","command":"m","error":"e"}],` +
		`"results":[],"leftOut":[],"manual":[],"failures":[],"notReached":[],"warnings":[],"error":""}`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

type subject struct{ id, label, dir string }

func (s subject) ID() string    { return s.id }
func (s subject) Label() string { return s.label }
func (s subject) Dir() string   { return s.dir }

func TestJoinNotesSkipsEmptyNotes(t *testing.T) {
	if got := JoinNotes("", "a", "", "b"); got != "a; b" {
		t.Errorf("got %q", got)
	}
	if got := JoinNotes("", ""); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestEntriesCarryTheSubject(t *testing.T) {
	s := subject{"abc", "api", "/work/api"}
	if got, want := Result(s, "stopped", ", which stopped meanwhile"), (Entry{
		ID: "abc", Name: "api", Cwd: "/work/api", Result: "stopped", Note: "which stopped meanwhile",
	}); got != want {
		t.Errorf("Result: %+v, want %+v", got, want)
	}
	if got, want := (Failure{Label: "api", Reason: "boom", Of: s}).Entry(), (Entry{
		ID: "abc", Name: "api", Cwd: "/work/api", Error: "boom",
	}); got != want {
		t.Errorf("a session's failure: %+v, want %+v", got, want)
	}
	if got, want := (Failure{Label: "park file", Reason: "locked"}).Entry(), (Entry{Name: "park file", Error: "locked"}); got != want {
		t.Errorf("a failure of no session: %+v, want %+v", got, want)
	}
	if got := Labels([]subject{s, {label: "web"}}); got != "api, web" {
		t.Errorf("Labels: %q", got)
	}
}

func TestPlanTableLeavesOutTheColumnsNoRowFills(t *testing.T) {
	rows := []PlanRow{{ID: "abc", Name: "api", Label: "idle", Cwd: "/work/api"}, {Name: "docs", Cwd: "/work/docs", Note: "last active now"}}
	table := PlanTable("STATUS", rows)
	if want := []string{"ID", "STATUS", "DIR", "NAME", "NOTE"}; !slices.Equal(table[0], want) {
		t.Errorf("header %q, want %q", table[0], want)
	}
	if want := []string{"-", "", "/work/docs", "docs", "last active now"}; !slices.Equal(table[2], want) {
		t.Errorf("row %q, want %q", table[2], want)
	}
	table = PlanTable("KIND", []PlanRow{{ID: "abc", Name: "api", Cwd: "/work/api", Size: "1.0K"}})
	if want := []string{"ID", "SIZE", "DIR", "NAME"}; !slices.Equal(table[0], want) {
		t.Errorf("header %q, want %q", table[0], want)
	}
	if got, want := rows[1].Entry(), (Entry{Name: "docs", Cwd: "/work/docs", Note: "last active now"}); got != want {
		t.Errorf("plan entry %+v, want %+v", got, want)
	}
	if got := (PlanRow{Note: "", Size: "2.0M"}).Entry().Note; got != "2.0M" {
		t.Errorf("a plan entry's note is %q, want its size", got)
	}
}
