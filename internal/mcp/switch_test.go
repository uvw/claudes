package mcp

import (
	"slices"
	"strings"
	"testing"
)

func TestPlanLeavesWhatIsAlreadyAsAsked(t *testing.T) {
	var s Switch
	s.Plan(map[string]string{"web": On, "browser": Off, "docs": Off}, []string{"docs", "web"})
	want := []Change{
		{Name: "browser", From: On, To: Off},
		{Name: "docs", From: Off, To: Off, Result: Already, Note: "already off"},
		{Name: "web", From: Off, To: On},
	}
	if !slices.Equal(s.Servers, want) {
		t.Fatalf("got %+v", s.Servers)
	}
	if !s.Pending() {
		t.Fatal("nothing pending")
	}
	if got := s.Apply([]string{"docs", "web"}); !slices.Equal(got, []string{"docs", "browser"}) {
		t.Fatalf("applied %q", got)
	}
	s.Mark(Planned, "")
	if s.Pending() || s.Servers[0].Result != Planned || s.Servers[1].Result != Already || s.Failed() {
		t.Fatalf("marked %+v", s.Servers)
	}
}

func TestVerifyChecksEachServerTheCommandActedOn(t *testing.T) {
	var s Switch
	s.Plan(map[string]string{"a": Off, "b": Off, "c": On}, nil)
	s.Verify([]string{"a"}, func(c Change) bool { return c.Name != "c" }, "not there")
	want := []Change{
		{Name: "a", From: On, To: Off, Result: Done},
		{Name: "b", From: On, To: Off, Result: Unchanged, Note: "not there"},
		{Name: "c", From: On, To: On, Result: Already, Note: "already on"},
	}
	if !slices.Equal(s.Servers, want) || !s.Failed() {
		t.Fatalf("got %+v", s.Servers)
	}
}

func TestMatchTakesGlobsAndExactNames(t *testing.T) {
	names := []string{"browser", "sql-main", "sql-test", "plugin:docs:search"}
	got, err := Match([]string{"sql-*", "browser", "sql-main", "plugin:docs:search"}, names)
	if err != nil || !slices.Equal(got, []string{"sql-main", "sql-test", "browser", "plugin:docs:search"}) {
		t.Fatalf("got %q, %v", got, err)
	}
	for pattern, want := range map[string]string{"nope*": "Servers: browser, sql-main", "[": "not a valid pattern"} {
		if _, err := Match([]string{pattern}, names); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: %v", pattern, err)
		}
	}
	if _, err := Match([]string{"x"}, nil); err == nil || !strings.Contains(err.Error(), "has none") {
		t.Fatalf("got %v", err)
	}
}

func TestProfileWantKeepsOnlyTheProfilesServers(t *testing.T) {
	want, missing := ProfileWant([]Server{{Name: "browser"}, {Name: "web"}}, []string{"web", "gone"})
	if len(want) != 2 || want["browser"] != Off || want["web"] != On || !slices.Equal(missing, []string{"gone"}) {
		t.Fatalf("want %v, missing %q", want, missing)
	}
}

func TestVerbAndWant(t *testing.T) {
	if Verb(On) != "enable" || Verb(Off) != "disable" {
		t.Fatal(Verb(On), Verb(Off))
	}
	if got := Want([]string{"a", "b"}, Off); len(got) != 2 || got["a"] != Off || got["b"] != Off {
		t.Fatalf("got %v", got)
	}
}
