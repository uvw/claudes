package jobs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func newStore(t *testing.T) Store {
	t.Helper()
	return Store{Dir: filepath.Join(t.TempDir(), "jobs"), LockAttempts: 3, LockDelay: time.Millisecond}
}

// writeJobFile writes name in job id's directory, or in the jobs directory when id
// is empty, and returns its path.
func writeJobFile(t *testing.T, s Store, id, name, data string) string {
	t.Helper()
	path := filepath.Join(s.Dir, id, name)
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustPin(t *testing.T, s Store, ids ...string) {
	t.Helper()
	if _, err := s.SetPinned(ids, true); err != nil {
		t.Fatal(err)
	}
}

func backdate(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestMissingPinsFileHasNoPins(t *testing.T) {
	pins, err := newStore(t).Pins()
	if err != nil || pins != nil {
		t.Fatalf("pins %v, err %v", pins, err)
	}
}

func TestPinWritesTheFileTheWayClaudeCodeDoes(t *testing.T) {
	s := newStore(t)
	if _, err := s.SetPinned([]string{"abf00936", "242e8b07"}, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, "pins.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := "[\n  \"abf00936\",\n  \"242e8b07\"\n]"
	if string(data) != want {
		t.Fatalf("pins.json\n got: %q\nwant: %q", data, want)
	}
}

func TestPinKeepsOrderAndReportsWhatChanged(t *testing.T) {
	s := newStore(t)
	mustPin(t, s, "aaaaaaaa")
	changed, err := s.SetPinned([]string{"aaaaaaaa", "bbbbbbbb"}, true)
	if err != nil || !slices.Equal(changed, []string{"bbbbbbbb"}) {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	pins, _ := s.Pins()
	if !slices.Equal(pins, []string{"aaaaaaaa", "bbbbbbbb"}) {
		t.Fatalf("pins %v", pins)
	}
}

func TestUnpinRemovesTheOrderFile(t *testing.T) {
	s := newStore(t)
	mustPin(t, s, "aaaaaaaa")
	order := writeJobFile(t, s, "aaaaaaaa", "order", "1")

	if _, err := s.SetPinned([]string{"aaaaaaaa"}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(order); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("order file still there: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "pins.json"))
	if string(data) != "[]" {
		t.Fatalf("pins.json %q", data)
	}
}

func TestAFreshLockIsRespected(t *testing.T) {
	s := newStore(t)
	mkdir(t, filepath.Join(s.Dir, "pins.json.lock"))

	if _, err := s.SetPinned([]string{"aaaaaaaa"}, true); err == nil {
		t.Fatal("wrote while another process held the lock")
	}
}

func TestAStaleLockIsTakenOver(t *testing.T) {
	s := newStore(t)
	lock := filepath.Join(s.Dir, "pins.json.lock")
	mkdir(t, lock)
	backdate(t, lock, time.Now().Add(-time.Minute))

	if _, err := s.SetPinned([]string{"aaaaaaaa"}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestACorruptPinsFileIsNotOverwritten(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "", "pins.json", "{oops")

	if _, err := s.SetPinned([]string{"aaaaaaaa"}, true); err == nil {
		t.Fatal("overwrote a corrupt pins.json")
	}
	data, _ := os.ReadFile(filepath.Join(s.Dir, "pins.json"))
	if string(data) != "{oops" {
		t.Fatalf("pins.json changed to %q", data)
	}
}

func TestVersionReadsTheJobState(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "aaaaaaaa", "state.json", `{"cliVersion":"2.1.322","name":"x"}`)

	if v := s.Version("aaaaaaaa"); v != "2.1.322" {
		t.Fatalf("version %q", v)
	}
	if v := s.Version("missing0"); v != "" {
		t.Fatalf("missing job version %q", v)
	}
}

func TestDefaultStoreRefusesARelativeConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "relative")
	if _, err := DefaultStore(); err == nil {
		t.Fatal("accepted a relative CLAUDE_CONFIG_DIR")
	}
	abs := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", abs)
	if s, err := DefaultStore(); err != nil || s.Dir != filepath.Join(abs, "jobs") {
		t.Fatalf("dir %q, err %v", s.Dir, err)
	}
}

func TestLastActiveIsWhenStateJSONChanged(t *testing.T) {
	s := newStore(t)
	if !s.LastActive("abf00936").IsZero() {
		t.Fatal("unknown job has a time")
	}
	when := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	backdate(t, writeJobFile(t, s, "abf00936", "state.json", "{}"), when)
	if got := s.LastActive("abf00936"); !got.Equal(when) {
		t.Fatalf("last active %v", got)
	}
}

func TestDefaultStoreFallsBackToTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if s, err := DefaultStore(); err != nil || s.Dir != filepath.Join(home, ".claude", "jobs") {
		t.Fatalf("dir %q, err %v", s.Dir, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if s, err := DefaultStore(); err == nil {
		t.Fatalf("guessed %q with no home directory", s.Dir)
	}
}

func TestPinSet(t *testing.T) {
	s := newStore(t)
	mustPin(t, s, "aaaaaaaa", "bbbbbbbb")
	set, err := s.PinSet()
	if err != nil || len(set) != 2 || !set["aaaaaaaa"] || !set["bbbbbbbb"] {
		t.Fatalf("set %v, err %v", set, err)
	}
}

func TestAnUnreadablePinsFileIsAnErrorNotNoPins(t *testing.T) {
	for name, setup := range map[string]func(path string) error{
		"corrupt":   func(path string) error { return os.WriteFile(path, []byte(`{"a":1}`), 0o644) },
		"directory": func(path string) error { return os.Mkdir(path, 0o755) },
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore(t)
			mkdir(t, s.Dir)
			if err := setup(filepath.Join(s.Dir, "pins.json")); err != nil {
				t.Fatal(err)
			}
			if set, err := s.PinSet(); err == nil {
				t.Fatalf("pins %v with no error", set)
			}
		})
	}
}

func TestUnpinningWhatIsNotPinnedWritesNothing(t *testing.T) {
	s := newStore(t)
	changed, err := s.SetPinned([]string{"aaaaaaaa"}, false)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed %v, err %v", changed, err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "pins.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("created pins.json: %v", err)
	}
}

func TestUnpinReportsAnOrderFileItCannotRemove(t *testing.T) {
	s := newStore(t)
	mustPin(t, s, "aaaaaaaa")
	// A directory with something in it cannot be removed like the order file.
	mkdir(t, filepath.Join(s.Dir, "aaaaaaaa", "order", "x"))
	if _, err := s.SetPinned([]string{"aaaaaaaa"}, false); err == nil {
		t.Fatal("no error for an order file that is still there")
	}
}

func TestVersionAndLastActiveOfUnknownJobs(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "aaaaaaaa", "state.json", "{not json")
	if v := s.Version("aaaaaaaa"); v != "" {
		t.Fatalf("version %q from a corrupt state.json", v)
	}
	if v := s.Version(""); v != "" {
		t.Fatalf("version %q for no job id", v)
	}
	if !s.LastActive("").IsZero() {
		t.Fatal("a time for no job id")
	}
}

func TestFolderBelongsToItsSession(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "aaaaaaaa", "state.json", `{"sessionId":"aaaaaaaa-1","cwd":"/work"}`)
	if got := s.Folder("aaaaaaaa", "aaaaaaaa-1"); got != "/work" {
		t.Fatalf("directory %q", got)
	}
	if got := s.Folder("aaaaaaaa", "other"); got != "" {
		t.Fatalf("another session's directory %q", got)
	}
}

func TestOptionsBelongToTheirSession(t *testing.T) {
	s := newStore(t)
	writeJobFile(t, s, "aaaaaaaa", "state.json", `{"sessionId":"aaaaaaaa-1","respawnFlags":["--model","m","--permission-mode","plan"]}`)
	writeJobFile(t, s, "bbbbbbbb", "state.json", `{"sessionId":"bbbbbbbb-1"}`)
	if got, ok := s.Options("aaaaaaaa", "aaaaaaaa-1"); !ok || !slices.Equal(got, []string{"--model", "m", "--permission-mode", "plan"}) {
		t.Fatalf("options %q, %v", got, ok)
	}
	if got, ok := s.Options("bbbbbbbb", "bbbbbbbb-1"); !ok || len(got) != 0 {
		t.Fatalf("a job with no saved options has %q, %v", got, ok)
	}
	for _, c := range [][2]string{{"aaaaaaaa", "other"}, {"cccccccc", "cccccccc-1"}, {"../x", "x"}} {
		if got, ok := s.Options(c[0], c[1]); ok || got != nil {
			t.Fatalf("Options(%q, %q) = %q, %v", c[0], c[1], got, ok)
		}
	}
}
