package teams

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestRowJoinsATeamWithItsLead(t *testing.T) {
	team := Team{Name: "session-62d022d3", LeadSessionID: lead, Members: []Member{{Name: "lead", Cwd: "/work"}}}
	r := RowOf(team, claude.Session{}, false)
	if r.LeadID != "62d022d3" || r.Cwd != "/work" || r.Running || r.Label() != "62d022d3" || r.ID() != "62d022d3" {
		t.Errorf("unknown lead: %+v", r)
	}
	s := claude.Session{JobID: "abcd1234", SessionID: lead, Name: "api", Cwd: "/api", PID: 9}
	r = RowOf(team, s, true)
	if r.LeadID != "abcd1234" || r.Cwd != "/api" || !r.Running || r.Label() != "api" {
		t.Errorf("known lead: %+v", r)
	}
	if !slices.Equal(r.Aliases(), []string{"api", "abcd1234", lead, "session-62d022d3"}) {
		t.Errorf("aliases %q", r.Aliases())
	}
}

func TestTaskSummaryCountsDoneAndNamesWhoWorks(t *testing.T) {
	if got := TaskSummary(nil); got != "-" {
		t.Errorf("no tasks: %q", got)
	}
	tasks := []Task{{ID: "1", Status: "completed"}, {ID: "2", Status: "in_progress", Owner: "tester"}, {ID: "3", Status: "in_progress"}, {ID: "4"}}
	if got := TaskSummary(tasks); got != "1/4 done, working: tester, #3" {
		t.Errorf("summary %q", got)
	}
}

func TestLeftoverSaysWhatItIs(t *testing.T) {
	st := store(t)
	team := Leftover{Path: filepath.Join(st.Teams, "session-62d022d3"), Name: "session-62d022d3"}
	tasks := Leftover{Path: filepath.Join(st.Tasks, "session-62d022d3"), Name: "session-62d022d3"}
	if team.ID() != "62d022d3" || team.Label() != "team" || tasks.Label() != "task list" || team.Dir() != team.Path {
		t.Errorf("team %s %s, tasks %s", team.ID(), team.Label(), tasks.Label())
	}
}
