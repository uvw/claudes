package target

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type ref struct{ name, id string }

func (r ref) Label() string     { return r.name }
func (r ref) ID() string        { return r.id }
func (r ref) Aliases() []string { return []string{r.name, r.id} }

var sessions = []ref{
	{"db-upgrade-plan", "242e8b07"},
	{"code-reviewer", "abf00936"},
	{"trends-review", "bd4d7315"},
	{"review", "11111111"},
	{"xbd4dy", "22222222"},
}

func names(rs []ref) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.name)
	}
	return strings.Join(out, ",")
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		tokens []string
		want   string
	}{
		{[]string{"code-reviewer"}, "code-reviewer"},
		{[]string{"242e8b07"}, "db-upgrade-plan"},
		{[]string{"242e"}, "db-upgrade-plan"},
		{[]string{"DB"}, "db-upgrade-plan"},
		{[]string{"trends"}, "trends-review"},
		{[]string{"review"}, "review"},
		{[]string{"db", "242e8b07", "code"}, "db-upgrade-plan,code-reviewer"},
		{[]string{"bd4d"}, "trends-review"},
		{[]string{"revi"}, "review"},
	} {
		got, err := Resolve(tc.tokens, sessions)
		if err != nil || names(got) != tc.want {
			t.Errorf("%v: got %q, err %v; want %q", tc.tokens, names(got), err, tc.want)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	for _, tc := range []struct{ token, want string }{
		{"view", `"view" matches 3 sessions`},
		{"nothing", `no session matches "nothing"`},
	} {
		_, err := Resolve([]string{tc.token}, sessions)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err %v; want %q", tc.token, err, tc.want)
		}
	}
}

func TestAMissNamesTheTargetThatMatchedNothing(t *testing.T) {
	var miss *MatchError
	_, err := Resolve([]string{"db", "carry on"}, sessions)
	if !errors.As(err, &miss) || !miss.NoMatch || miss.Missed != "carry on" {
		t.Fatalf("err %#v", err)
	}
	_, err = Match([]string{"nothing"}, sessions)
	if !errors.As(err, &miss) || !miss.NoMatch || miss.Missed != "nothing" {
		t.Fatalf("err %#v", err)
	}
	_, err = Resolve([]string{"view"}, sessions)
	if !errors.As(err, &miss) || miss.NoMatch || miss.Missed != "" {
		t.Fatalf("an ambiguous target is a miss: %#v", err)
	}
}

func TestMatchPatterns(t *testing.T) {
	for _, tc := range []struct {
		tokens []string
		want   string
	}{
		{[]string{"*review*"}, "code-reviewer,trends-review,review"},
		{[]string{"DB*"}, "db-upgrade-plan"},
		{[]string{"242e*"}, "db-upgrade-plan"},
		{[]string{"?review"}, ""},
	} {
		got, err := Match(tc.tokens, sessions)
		if tc.want == "" {
			if err == nil {
				t.Errorf("%v: matched %q", tc.tokens, names(got))
			}
			continue
		}
		if err != nil || names(got) != tc.want {
			t.Errorf("%v: got %q, err %v; want %q", tc.tokens, names(got), err, tc.want)
		}
	}
}

func TestMatchWordsArePartOfANameOrTheStartOfAnID(t *testing.T) {
	for word, want := range map[string]string{
		"REVIEW": "code-reviewer,trends-review,review",
		"plan":   "db-upgrade-plan",
		"abf0":   "code-reviewer",
		"bd4d":   "trends-review,xbd4dy",
	} {
		if got, err := Match([]string{word}, sessions); err != nil || names(got) != want {
			t.Errorf("%s: got %q, err %v; want %q", word, names(got), err, want)
		}
	}
	if got, err := Match([]string{"f009"}, sessions); err == nil {
		t.Fatalf("f009 matched the middle of an id: %q", names(got))
	}
}

func TestAShortWordNeverMatchesAnID(t *testing.T) {
	agents := []ref{{"agent-2", "11111111"}, {"agent-7", "2c4d9e01"}, {"prod-db-migration", "33333333"}, {"frontend", "db12ab34"}}
	for word, want := range map[string]string{"2": "agent-2", "db": "prod-db-migration", "db12": "frontend"} {
		got, err := Resolve([]string{word}, agents)
		if err != nil || names(got) != want {
			t.Errorf("Resolve %s: got %q, err %v; want %q", word, names(got), err, want)
		}
		if got, err := Match([]string{word}, agents); err != nil || names(got) != want {
			t.Errorf("Match %s: got %q, err %v; want %q", word, names(got), err, want)
		}
	}
}

func TestMatchDropsDuplicates(t *testing.T) {
	if got, err := Match([]string{"db", "242e8b07"}, sessions); err != nil || names(got) != "db-upgrade-plan" {
		t.Fatalf("got %q, %v", names(got), err)
	}
}

func TestMatchErrors(t *testing.T) {
	for token, want := range map[string]string{"nothing": `no session matches "nothing"`, "[abc": "bad pattern"} {
		if _, err := Match([]string{token}, sessions); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v; want %q", token, err, want)
		}
	}
}

func TestAMissListsAtMostTenSessions(t *testing.T) {
	var many []ref
	for i := range 13 {
		many = append(many, ref{name: fmt.Sprintf("s%02d", i), id: fmt.Sprintf("id%02d", i)})
	}
	_, err := Resolve([]string{"nope"}, many)
	if err == nil || !strings.Contains(err.Error(), "s09 (id09), and 3 more") || strings.Contains(err.Error(), "s10") {
		t.Fatalf("err %v", err)
	}
}

func TestAnEmptyTargetNamesNoSession(t *testing.T) {
	if _, err := Resolve([]string{" "}, sessions); err == nil || !strings.Contains(err.Error(), "empty target") {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := Match([]string{"x", ""}, sessions); err == nil || !strings.Contains(err.Error(), "empty target") {
		t.Fatalf("Match: %v", err)
	}
}
