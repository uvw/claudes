package history

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/transcripts"
)

const week = 7 * 24 * time.Hour

// id is a valid session id made of the digit d.
func id(d string) string {
	return strings.Repeat(d, 8) + "-" + strings.Repeat(d, 4) + "-" + strings.Repeat(d, 4) + "-" +
		strings.Repeat(d, 4) + "-" + strings.Repeat(d, 12)
}

// file writes data to path, and sets when the file last changed.
func file(t *testing.T, path, data string, changed time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
}

// past is a session of the given id, last active at active, whose transcript
// is in the project directory proj under st.
func past(t *testing.T, st transcripts.Store, d string, active time.Time) Session {
	t.Helper()
	path := filepath.Join(st.Dir, "proj", id(d)+".jsonl")
	file(t, path, "{}\n", active)
	cwd := t.TempDir()
	return Session{ShortID: id(d)[:8], Session: transcripts.Session{
		SessionID: id(d), Name: "s" + d, Cwd: cwd, Path: path, LastActive: active, Size: 3,
	}, Disk: transcripts.Disk{Total: 3, Transcript: 3}}
}

func TestPickTakesTheNewestOfANameAndRefusesAnAmbiguousToken(t *testing.T) {
	list := []Session{
		{ShortID: "aaaaaaaa", Session: transcripts.Session{SessionID: id("a"), Name: "api"}},
		{ShortID: "bbbbbbbb", Session: transcripts.Session{SessionID: id("b"), Name: "api"}},
		{ShortID: "cccccccc", Session: transcripts.Session{SessionID: id("c"), Name: "docs"}},
		{ShortID: "dddddddd", Session: transcripts.Session{SessionID: id("d"), Name: "docs-old"}},
	}
	for token, want := range map[string]string{"api": id("a"), "cccc": id("c"), "ap": id("a")} {
		p, err := Pick(token, list)
		if err != nil || p.SessionID != want {
			t.Errorf("%s: picked %s, err %v, want %s", token, p.SessionID, err, want)
		}
	}
	if _, err := Pick("doc", list); err == nil {
		t.Error("doc picked one of two sessions with different names")
	}
	if sameName(nil) || sameName(list[2:]) || !sameName(list[:2]) {
		t.Error("sameName")
	}
}

// A transcript claudes could not read is kept for that reason, not for being
// too big to read, and is read only once.
func TestTriageSaysWhenItCannotReadATranscript(t *testing.T) {
	p := Session{Session: transcripts.Session{Path: filepath.Join(t.TempDir(), "gone.jsonl"), Size: 10}}
	p.ReadTally(false)
	if p.Activity != nil || p.tallyErr == nil {
		t.Fatalf("activity %+v, err %v", p.Activity, p.tallyErr)
	}
	if why := keepWhy(p); why != "cannot read it" {
		t.Fatalf("kept as %q", why)
	}
	first := p.tallyErr
	p.ReadTally(true)
	if !errors.Is(p.tallyErr, first) {
		t.Fatal("read the transcript a second time")
	}
}

func TestReadTallySkipsABigTranscriptUnlessForced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.jsonl")
	file(t, path, `{"type":"user","message":{"content":"hi"}}`+"\n", time.Now())
	p := Session{Session: transcripts.Session{Path: path, Size: transcripts.TallyLimit + 1}}
	p.ReadTally(false)
	if p.Activity != nil || keepWhy(p) != "bigger than "+render.Size(transcripts.TallyLimit) {
		t.Fatalf("read a big transcript: %+v", p.Activity)
	}
	p.ReadTally(true)
	if p.Activity == nil {
		t.Fatal("force did not read it")
	}
}

func TestKeepSaysWhySessionGcLeavesEachAlone(t *testing.T) {
	list := []Session{
		{Session: transcripts.Session{SessionID: id("1")}, Running: true},
		{Session: transcripts.Session{SessionID: id("2")}},
		{Session: transcripts.Session{SessionID: id("3")}},
		{Session: transcripts.Session{SessionID: id("4")}},
		{Session: transcripts.Session{SessionID: id("5")}},
	}
	listed := []claude.Session{{SessionID: id("1")}, {SessionID: id("2")}, {SessionID: id("3"), PID: 7}}
	keep := Keep(list, listed, []string{id("4")}, func(s string) bool { return s == id("5") })
	want := map[string]string{
		id("1"): "is running",
		id("2"): "is still in agent view's list (claudes gc removes it from the list)",
		id("3"): "is running",
		id("4"): "is parked",
		id("5"): "is the session running claudes",
	}
	if len(keep) != len(want) {
		t.Fatalf("keep %v", keep)
	}
	for k, v := range want {
		if keep[k] != v {
			t.Errorf("%s: %q, want %q", k[:8], keep[k], v)
		}
	}
}

func TestCollectPicksEachKind(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * week)
	st := transcripts.Store{Dir: t.TempDir()}

	empty := past(t, st, "1", old)
	empty.Empty = true
	trivial := past(t, st, "2", old)
	trivial.Activity = &transcripts.Tally{Prompts: 1}
	gone := past(t, st, "3", old)
	gone.Cwd = filepath.Join(gone.Cwd, "gone")
	stale := past(t, st, "4", old)
	recent := past(t, st, "5", now)
	recent.Empty = true
	kept := past(t, st, "6", old)
	kept.Empty = true
	copied := past(t, st, "7", old)
	copyPath := filepath.Join(st.Dir, "other", id("7")+".jsonl")
	file(t, copyPath, "{}\n", old)
	copied.Copies = []string{copyPath}
	sidecar := past(t, st, "8", old)
	file(t, filepath.Join(transcripts.Sidecar(sidecar.Session), "tool-results", "r.txt"), "12345", old)
	leftover := filepath.Join(st.Dir, "proj", id("9"))
	file(t, filepath.Join(leftover, "r.txt"), "1234", old)
	if err := os.Chtimes(leftover, old, old); err != nil {
		t.Fatal(err)
	}
	list := []Session{empty, trivial, gone, stale, recent, kept, copied, sidecar}
	keep := map[string]string{id("6"): "is parked"}

	for _, tc := range []struct {
		kinds Kinds
		want  []string
	}{
		{Kinds{Empty: true}, []string{"empty s1"}},
		{Kinds{Trivial: true}, []string{"trivial s2"}},
		{Kinds{Gone: true}, []string{"gone s3"}},
		{Kinds{OlderThan: week}, []string{"old s1", "old s2", "old s3", "old s4", "old s7", "old s8"}},
		{Kinds{Copies: true}, []string{"copy s7 (older copy)"}},
		{Kinds{Sidecars: true}, []string{"sidecar s8 (saved outputs)"}},
		{Kinds{Leftovers: true}, []string{"leftover sidecar " + id("9")[:8]}},
		{Kinds{Empty: true, Sidecars: true}, []string{"empty s1", "sidecar s8 (saved outputs)"}},
	} {
		items, err := Collect(st, slices.Clone(list), keep, tc.kinds, now)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, it := range items {
			got = append(got, string(it.Why)+" "+it.Name)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%+v: got %q, want %q", tc.kinds, got, tc.want)
		}
	}
}

func TestItemsNameTheirDirectoryAndOnlyWholeSessionsAnID(t *testing.T) {
	st := transcripts.Store{Dir: t.TempDir()}
	p := past(t, st, "1", time.Now())
	whole := Whole(st, p, KindNamed)
	if e := render.EntryOf(whole); e.ID != "11111111" || e.Name != "s1" || e.Cwd != p.Cwd {
		t.Errorf("whole session: %+v", e)
	}
	if whole.Transcript != p.Path || whole.Changed.IsZero() || !slices.Contains(whole.Paths, p.Path) {
		t.Errorf("whole session: %+v", whole)
	}
	for _, why := range []Kind{KindCopy, KindSidecar, KindLeftover} {
		if id := (Item{SessionID: p.SessionID, Why: why}).ID(); id != "" {
			t.Errorf("%s has id %q", why, id)
		}
	}
}

func TestTrivialNeedsAWholeTranscriptWithOnePromptAndNoWork(t *testing.T) {
	for _, tc := range []struct {
		p    Session
		want bool
	}{
		{Session{Activity: &transcripts.Tally{Prompts: 1}}, true},
		{Session{Activity: &transcripts.Tally{Commands: 2}}, true},
		{Session{}, false},
		{Session{Activity: &transcripts.Tally{Prompts: 2}}, false},
		{Session{Activity: &transcripts.Tally{Prompts: 1, ToolCalls: 1}}, false},
		{Session{Activity: &transcripts.Tally{Prompts: 1}, Session: transcripts.Session{NameSource: transcripts.NameCustom}}, false},
		{Session{Activity: &transcripts.Tally{Prompts: 1}, Disk: transcripts.Disk{FileHistory: 1}}, false},
	} {
		if got := trivial(tc.p); got != tc.want {
			t.Errorf("%+v: trivial %v", tc.p.Activity, got)
		}
	}
}

func TestTriageSortsIntoRemoveReviewAndKeep(t *testing.T) {
	now := time.Now()
	old := now.Add(-2 * week)
	st := transcripts.Store{Dir: t.TempDir()}
	empty := past(t, st, "1", old)
	empty.Empty = true
	review := past(t, st, "2", old)
	review.Activity = &transcripts.Tally{Prompts: 1, ToolCalls: 3}
	busy := past(t, st, "3", old)
	busy.Activity = &transcripts.Tally{Prompts: 2}
	parked := past(t, st, "4", old)
	fresh := past(t, st, "5", now)
	list := []Session{empty, review, busy, parked, fresh}
	keep := map[string]string{id("4"): "is parked"}

	items, err := Collect(st, slices.Clone(list), keep, Kinds{Empty: true, Trivial: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	r := Triage(list, items, keep, now)
	whys := func(items []TriageItem) (out []string) {
		for _, t := range items {
			out = append(out, t.Name+": "+t.Why)
		}
		return out
	}
	if got := whys(r.Remove); !slices.Equal(got, []string{"s1: empty"}) {
		t.Errorf("remove %q", got)
	}
	if got := whys(r.Review); !slices.Equal(got, []string{"s2: review"}) {
		t.Errorf("review %q", got)
	}
	if got := whys(r.Keep); !slices.Equal(got, []string{"s3: more than one prompt", "s4: parked", "s5: changed in the last day"}) {
		t.Errorf("keep %q", got)
	}
	if TriageSize(r.Keep) != 9 {
		t.Errorf("keep size %d", TriageSize(r.Keep))
	}
}

func TestKeepWhyNamesWhatMakesASessionWorthKeeping(t *testing.T) {
	for want, p := range map[string]Session{
		"named with /rename":       {Activity: &transcripts.Tally{}, Session: transcripts.Session{NameSource: transcripts.NameCustom}},
		"started by an agent team": {Activity: &transcripts.Tally{Teammate: true}},
		"changed files":            {Activity: &transcripts.Tally{Edits: 1}},
		"more than 20 tool calls":  {Activity: &transcripts.Tally{Prompts: 1, ToolCalls: ReviewToolCalls + 1}},
		"no prompt it could read":  {Activity: &transcripts.Tally{}},
		"":                         {Activity: &transcripts.Tally{Prompts: 1}},
		"more than one prompt":     {Activity: &transcripts.Tally{Prompts: 3}},
	} {
		if got := keepWhy(p); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestGcCommandForNamesTheKindsOnlyWhenTheDefaultWouldNotDo(t *testing.T) {
	items := func(whys ...string) []TriageItem {
		var out []TriageItem
		for _, w := range whys {
			out = append(out, TriageItem{Why: w})
		}
		return out
	}
	for _, tc := range []struct {
		whys []string
		want string
	}{
		{[]string{"empty", "copy"}, "claudes session gc"},
		{[]string{"trivial"}, "claudes session gc --trivial"},
		{[]string{"leftover", "trivial", "empty"}, "claudes session gc --empty --trivial --leftovers"},
	} {
		if got := gcCommandFor(items(tc.whys...)); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.whys, got, tc.want)
		}
		if got := RemoveCommand(items(tc.whys...), false); got != tc.want {
			t.Errorf("%v: remove command %q, want %q", tc.whys, got, tc.want)
		}
	}
}

func TestRemoveCommandNamesEachSessionWhenNarrowed(t *testing.T) {
	a, b, c := TriageItem{Ref: Ref{ID: "aaaaaaaa"}}, TriageItem{Ref: Ref{ID: "bbbbbbbb"}}, TriageItem{Why: "copy"}
	for _, tc := range []struct {
		items []TriageItem
		want  string
	}{
		{[]TriageItem{a, b}, "claudes session gc aaaaaaaa bbbbbbbb"},
		{[]TriageItem{c}, "claudes session gc --copies"},
		{[]TriageItem{a, c}, "claudes session gc aaaaaaaa, and claudes session gc --copies for the older copies, which have no id"},
	} {
		if got := RemoveCommand(tc.items, true); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

func TestUsageGroupsByProjectAndBiggestTakesTheLargest(t *testing.T) {
	list := []Session{
		{ShortID: "aaaaaaaa", Session: transcripts.Session{Path: "/p/one/a.jsonl", Cwd: "/one"}, Disk: transcripts.Disk{Total: 10}},
		{ShortID: "bbbbbbbb", Session: transcripts.Session{Path: "/p/two/b.jsonl", Cwd: "/two"}, Disk: transcripts.Disk{Total: 30}},
		{ShortID: "cccccccc", Session: transcripts.Session{Path: "/p/one/c.jsonl", Cwd: "/one-old"}, Disk: transcripts.Disk{Total: 25}},
	}
	u := UsageOf(list, nil)
	if len(u.Dirs) != 2 || u.Dirs[0].Project != "one" || u.Dirs[0].Sessions != 2 || u.Dirs[0].Dir != "/one" || u.Total != 65 {
		t.Errorf("usage %+v", u)
	}
	if u.Leftovers == nil {
		t.Error("leftovers encode as null")
	}
	u = UsageOf(list, []transcripts.Leftover{{Size: 5}})
	if u.Total != 70 {
		t.Errorf("total %d with a leftover", u.Total)
	}
	top := Biggest(list, 2)
	if len(top) != 2 || top[0].ID != "bbbbbbbb" || top[1].ID != "cccccccc" {
		t.Errorf("biggest %+v", top)
	}
	if len(Biggest(list, 0)) != 0 || Biggest(list, 0) == nil || len(Biggest(list, 9)) != 3 {
		t.Error("biggest at the edges")
	}
	cells := UsageCells(transcripts.Disk{Total: 2048}, "3", "/one")
	if len(cells) != 7 || cells[5] != "3" || cells[6] != "/one" || cells[0] != render.Size(2048) {
		t.Errorf("cells %q", cells)
	}
}

func TestDetailsShowWhatTheTranscriptSays(t *testing.T) {
	now := time.Now()
	p := Session{
		Session: transcripts.Session{SessionID: id("1"), Name: "api", NameSource: transcripts.NameCustom, LastActive: now.Add(-time.Hour)},
		Running: true,
		Activity: &transcripts.Tally{
			Prompts: 1, Commands: 2, ToolCalls: 3, Edits: 0, Teammate: true, FirstPrompt: "fix it\nplease",
		},
	}
	got := map[string]string{}
	for _, kv := range Details(p, now) {
		got[kv[0]] = kv[1]
	}
	for k, want := range map[string]string{
		"name":         "api (set with /rename)",
		"state":        "running",
		"started":      "",
		"activity":     "1 prompt, 2 commands, 3 tool calls, 0 edits, started by an agent team's lead",
		"first prompt": "fix it …",
	} {
		if got[k] != want {
			t.Errorf("%s: %q, want %q", k, got[k], want)
		}
	}
	if !strings.HasSuffix(got["active"], "(1h ago)") {
		t.Errorf("active %q", got["active"])
	}
	for source, want := range map[string]string{
		transcripts.NameAgent: "x (from agent view)", transcripts.NameAI: "x (Claude Code's title)", "": "x",
	} {
		if got := nameWithSource("x", source); got != want {
			t.Errorf("%s: %q", source, got)
		}
	}
	if nameWithSource("", transcripts.NameCustom) != "" || activity(nil) != "" || firstPrompt(nil) != "" {
		t.Error("an unknown value is not empty")
	}
}

func TestHitsMaskSecretsInNamesUnlessRaw(t *testing.T) {
	results := []transcripts.Result{{Session: transcripts.Session{SessionID: id("1"), Name: "GITHUB_TOKEN=" + strings.Repeat("x7", 12)}, Score: 2}}
	if h := Hits(results, false); len(h) != 1 || h[0].ID != "11111111" || strings.Contains(h[0].Name, "x7x7") || h[0].Score != 2 {
		t.Errorf("hits %+v", h)
	}
	if h := Hits(results, true); h[0].Name != results[0].Name {
		t.Errorf("raw hits %+v", h)
	}
}
