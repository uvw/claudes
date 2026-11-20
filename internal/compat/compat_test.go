package compat

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

func TestCheckVersion(t *testing.T) {
	r := Range{Min: "2.1.300", Max: "2.1.325"}
	for v, warns := range map[string]bool{
		"":        false,
		"2.1.300": false,
		"2.1.325": false,
		"2.1.999": false,
		"2.1.299": true,
		"2.2.0":   true,
		"3.0.0":   true,
	} {
		if got := r.CheckVersion(v) != ""; got != warns {
			t.Errorf("CheckVersion(%q) warns %v, want %v", v, got, warns)
		}
	}
	if r.String() != "2.1.300 to 2.1.x" {
		t.Errorf("String() = %q", r)
	}
}

func TestCheckSessions(t *testing.T) {
	good := []claude.Session{
		{JobID: "3ec56511", SessionID: "3ec56511-0000-4000-8000-000000000001", Kind: "background", Status: "busy"},
		{SessionID: "e89ec2c4-0000-4000-8000-000000000002", Kind: "interactive"},
	}
	if p := CheckSessions(good); len(p) != 0 {
		t.Fatalf("problems in good sessions: %q", p)
	}
	bad := []claude.Session{
		{Name: "alpha", SessionID: "3ec56511-0000-4000-8000-000000000001", JobID: "3ec56511", Kind: "daemon", Status: "idle"},
		{Name: "beta", SessionID: "3ec56511-0000-4000-8000-000000000001", JobID: "3ec56511", Kind: "background", Status: "thinking"},
		{Name: "gamma", SessionID: "3ec56511-0000-4000-8000-000000000001", JobID: "job-1", Kind: "background"},
	}
	p := CheckSessions(bad)
	if odd := CheckSessions([]claude.Session{{SessionID: "x&y", Kind: "interactive"}}); len(odd) != 1 || !strings.Contains(odd[0], "not a UUID") {
		t.Fatalf("an odd session id passed: %q", odd)
	}
	if len(p) != 3 || !strings.Contains(p[0], `kind "daemon"`) || !strings.Contains(p[1], `status "thinking"`) || !strings.Contains(p[2], `"job-1"`) {
		t.Fatalf("problems %q", p)
	}
}

func TestCheckPins(t *testing.T) {
	if p := CheckPins([]string{"3ec56511", "ff78d735"}); len(p) != 0 {
		t.Fatalf("problems %q", p)
	}
	if p := CheckPins([]string{"3ec56511", "3EC56511", "{}"}); len(p) != 2 {
		t.Fatalf("problems %q", p)
	}
}

func TestCheckHelpPassesOnRealHelp(t *testing.T) {
	root, agents := readHelp(t)
	if missing := CheckHelp(root, agents); len(missing) != 0 {
		t.Fatalf("missing %q from Claude Code 2.1.325's help", missing)
	}
}

func TestCheckHelpFindsWhatIsGone(t *testing.T) {
	root, agents := readHelp(t)
	root = strings.Replace(root, "  stop|kill <id>", "  kill <id>", 1)
	root = strings.Replace(root, "  --bg, --background", "  --background", 1)
	agents = strings.Replace(agents, "  --json ", "  --format ", 1)
	got := strings.Join(CheckHelp(root, agents), ", ")
	if got != "claude stop, claude --bg, claude agents --json" {
		t.Fatalf("missing %q", got)
	}
}

func TestCheckHelpIgnoresMentionsInSentences(t *testing.T) {
	root := "Options:\n  --other   works with --resume and --bg\n\nCommands:\n  agents   lists stop and rm\n"
	want := []string{
		"claude attach", "claude logs", "claude respawn", "claude rm", "claude stop",
		"claude --bg", "claude --resume", "claude --version",
		"claude agents --json", "claude agents --all",
	}
	if got := CheckHelp(root, ""); !slices.Equal(got, want) {
		t.Fatalf("missing\n got: %q\nwant: %q", got, want)
	}
}

func readHelp(t *testing.T) (string, string) {
	t.Helper()
	root, err := os.ReadFile(filepath.Join("testdata", "help-2.1.325.txt"))
	if err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(filepath.Join("testdata", "agents-help-2.1.325.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(root), string(agents)
}

func TestCachedVersion(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	calls := 0
	lookup := func() (string, error) { calls++; return "2.1.325", nil }

	for _, at := range []time.Duration{0, time.Hour, 5 * time.Hour} {
		if v, err := CachedVersion(dir, "/bin/claude", now.Add(at), lookup); err != nil || v != "2.1.325" {
			t.Fatalf("v %q, err %v", v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("looked up %d times within six hours, want 1", calls)
	}
	if _, err := CachedVersion(dir, "/other/claude", now, lookup); err != nil || calls != 2 {
		t.Fatalf("another binary used the cache: calls %d, err %v", calls, err)
	}
	if _, err := CachedVersion(dir, "/other/claude", now.Add(7*time.Hour), lookup); err != nil || calls != 3 {
		t.Fatalf("a stale cache was used: calls %d, err %v", calls, err)
	}
}

func TestCachedVersionWithoutADirAndOnFailure(t *testing.T) {
	calls := 0
	lookup := func() (string, error) { calls++; return "2.1.325", nil }
	for range 2 {
		if _, err := CachedVersion("", "claude", time.Now(), lookup); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls %d, want 2 with the cache off", calls)
	}
	dir := t.TempDir()
	boom := errors.New("boom")
	if _, err := CachedVersion(dir, "claude", time.Now(), func() (string, error) { return "", boom }); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "claude-version.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a failed lookup was cached")
	}
}

func TestCachedVersionNoticesAClaudeUpdatedInPlace(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(binary, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	calls := 0
	lookup := func() (string, error) { calls++; return "2.1.325", nil }
	for range 2 {
		if _, err := CachedVersion(dir, binary, now, lookup); err != nil {
			t.Fatal(err)
		}
	}
	later := now.Add(time.Minute)
	if err := os.Chtimes(binary, later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedVersion(dir, binary, now, lookup); err != nil || calls != 2 {
		t.Fatalf("calls %d, err %v; want a fresh lookup after the update", calls, err)
	}
}
