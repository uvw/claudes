package teams

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

const lead = "62d022d3-7ca9-40c6-b1fe-326333f8cf67"

func store(t *testing.T) Store {
	dir := t.TempDir()
	return Store{Teams: filepath.Join(dir, "teams"), Tasks: filepath.Join(dir, "tasks")}
}

func write(t *testing.T, path, data string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

const config = `{"name":"session-62d022d3","leadSessionId":"` + lead + `","members":[
 {"name":"team-lead","agentType":"team-lead","backendType":"in-process"},
 {"name":"writer","agentType":"general-purpose","backendType":"in-process"},
 {"name":"tester","agentType":"general-purpose","backendType":"in-process"}]}`

func TestTheLeadsTeamNamesItsTeammates(t *testing.T) {
	st := store(t)
	write(t, filepath.Join(st.Teams, "session-62d022d3", "config.json"), config, time.Now())

	if diff := cmp.Diff([]string{"writer", "tester"}, st.Teammates(lead)); diff != "" {
		t.Fatalf("teammates (-want +got):\n%s", diff)
	}
	if got := st.Teammates("62d022d3-0000-4000-8000-000000000000"); len(got) != 0 {
		t.Fatalf("another session with the same short id got %q", got)
	}
	if got := (Store{}).Teammates(lead); len(got) != 0 {
		t.Fatalf("an unset store found %q", got)
	}
	teams, err := st.List()
	if err != nil || len(teams) != 1 || teams[0].Name != "session-62d022d3" {
		t.Fatalf("teams %+v, err %v", teams, err)
	}
}

func TestTaskListIsInIDOrder(t *testing.T) {
	st := store(t)
	dir := filepath.Join(st.Tasks, "session-62d022d3")
	write(t, filepath.Join(dir, "10.json"), `{"id":"10","subject":"ten","status":"pending"}`, time.Now())
	write(t, filepath.Join(dir, "2.json"), `{"id":"2","subject":"two","status":"completed","owner":"writer","blockedBy":["1"]}`, time.Now())
	write(t, filepath.Join(dir, "bad.json"), `{not json`, time.Now())

	tasks, err := st.TaskList("session-62d022d3")
	if err != nil {
		t.Fatal(err)
	}
	want := []Task{
		{ID: "2", Subject: "two", Status: "completed", Owner: "writer", BlockedBy: []string{"1"}},
		{ID: "10", Subject: "ten", Status: "pending", BlockedBy: []string{}},
	}
	if diff := cmp.Diff(want, tasks); diff != "" {
		t.Fatalf("tasks (-want +got):\n%s", diff)
	}
}

func TestLeftoversSkipRunningAndRecentSessions(t *testing.T) {
	st := store(t)
	old := time.Now().Add(-30 * 24 * time.Hour)
	write(t, filepath.Join(st.Teams, "session-aaaaaaaa", "config.json"), "{}", old)
	write(t, filepath.Join(st.Tasks, "session-aaaaaaaa", "1.json"), "{}", old)
	write(t, filepath.Join(st.Tasks, "session-bbbbbbbb", "1.json"), "{}", old)
	write(t, filepath.Join(st.Tasks, "session-cccccccc", "1.json"), "{}", time.Now())
	write(t, filepath.Join(st.Tasks, "other", "1.json"), "{}", old)

	got, err := st.Leftovers(map[string]bool{"bbbbbbbb": true}, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	want := []string{filepath.Join(st.Teams, "session-aaaaaaaa"), filepath.Join(st.Tasks, "session-aaaaaaaa")}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Fatalf("leftovers (-want +got):\n%s", diff)
	}
	for _, f := range got {
		if err := st.Remove(f, os.RemoveAll); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(f.Path); !os.IsNotExist(err) {
			t.Fatalf("%s is still there", f.Path)
		}
	}
	if err := st.Remove(Leftover{Path: t.TempDir()}, os.RemoveAll); err == nil {
		t.Fatal("removed a directory outside teams and tasks")
	}
}

func TestAnUnreadableTeamConfigCountsAsTeammates(t *testing.T) {
	st := Store{Teams: t.TempDir(), Tasks: t.TempDir()}
	const id = "11111111-2222-3333-4444-555555555555"
	dir := filepath.Join(st.Teams, Name(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := st.Teammates(id); len(got) != 1 || got[0] != Unknown {
		t.Fatalf("teammates %q", got)
	}
	if got := st.Teammates("99999999-2222-3333-4444-555555555555"); len(got) != 0 {
		t.Fatalf("a session with no team directory has teammates %q", got)
	}
	bad, err := st.Unreadable()
	if err != nil || len(bad) != 1 || bad[0] != Name(id) {
		t.Fatalf("unreadable %q, err %v", bad, err)
	}
}
