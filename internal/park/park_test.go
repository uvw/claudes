package park

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/uvw/claudes/internal/claude"
)

func TestRoundTripAndEmptyDeletesTheFile(t *testing.T) {
	f := File{Path: filepath.Join(t.TempDir(), "nested", "parked.json")}
	if entries, err := f.Load(); err != nil || entries != nil {
		t.Fatalf("missing file: %v %v", entries, err)
	}
	want := []Entry{{SessionID: "a-1", Name: "a", Cwd: "/x", Kind: "background", WasBusy: true, PausedAt: "t"}}
	if err := f.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := f.Load()
	if err != nil || len(got) != 1 || got[0] != want[0] {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if err := f.Save(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("file still there: %v", err)
	}
}

func TestMergeReplacesAndKeepsOrder(t *testing.T) {
	a1, b, a2 := Entry{SessionID: "a", Name: "old"}, Entry{SessionID: "b"}, Entry{SessionID: "a", Name: "new"}
	got := Merge([]Entry{a1, b}, []Entry{a2})
	if len(got) != 2 || got[0].Name != "new" || got[1].SessionID != "b" {
		t.Fatalf("merge %+v", got)
	}
}

func TestUpdateFromSeveralProcessesLosesNothing(t *testing.T) {
	f := File{Path: filepath.Join(t.TempDir(), "parked.json")}
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			err := f.Update(func(entries []Entry) []Entry {
				return Merge(entries, []Entry{{SessionID: fmt.Sprint(i)}})
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if entries, err := f.Load(); err != nil || len(entries) != 20 {
		t.Fatalf("%d entries, err %v", len(entries), err)
	}
}

func TestLoadRefusesAFileFromANewerClaudes(t *testing.T) {
	f := File{Path: filepath.Join(t.TempDir(), "parked.json")}
	if err := os.WriteFile(f.Path, []byte(`{"version": 2, "sessions": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Load(); err == nil || !strings.Contains(err.Error(), "written by a newer claudes") {
		t.Fatalf("err %v, want a newer park file refused", err)
	}
}

func TestAnEntryIsNamedByItsJobIDToo(t *testing.T) {
	e := Entry{SessionID: "deadbeef-1111", JobID: "7f8f7d77", Name: "job-one"}
	if !slices.Contains(e.Aliases(), "7f8f7d77") {
		t.Fatalf("aliases %v", e.Aliases())
	}
}

func TestDefaultFileIgnoresARelativeStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative")
	f, err := DefaultFile()
	if err != nil || !filepath.IsAbs(f.Path) {
		t.Fatalf("path %q, err %v", f.Path, err)
	}
}

func TestFromSessionRecordsWhatResumeNeeds(t *testing.T) {
	s := claude.Session{JobID: "7f8f7d77", SessionID: "deadbeef-1111", Cwd: "/x", Kind: "background", Status: "waiting"}
	e := FromSession(s, "2026-11-14T00:00:00Z")
	want := Entry{SessionID: s.SessionID, JobID: s.JobID, Name: "7f8f7d77", Cwd: "/x", Kind: "background", WasBusy: true, PausedAt: "2026-11-14T00:00:00Z"}
	if diff := cmp.Diff(want, e); diff != "" {
		t.Fatalf("entry (-want +got):\n%s", diff)
	}
	if e.Label() != "7f8f7d77" || e.Interactive() {
		t.Fatalf("an unnamed background session: label %q, interactive %v", e.Label(), e.Interactive())
	}
	if !strings.Contains(e.ManualCommand(), "--bg --resume deadbeef-1111") {
		t.Fatalf("command %q", e.ManualCommand())
	}
	tty := FromSession(claude.Session{SessionID: "cafe0000-2222", Name: "tty", Kind: "interactive", Status: "idle"}, "")
	if !tty.Interactive() || tty.WasBusy || strings.Contains(tty.ManualCommand(), "--bg") {
		t.Fatalf("interactive entry %+v, command %q", tty, tty.ManualCommand())
	}
}

func TestACorruptParkFileIsAnErrorAndIsKept(t *testing.T) {
	f := File{Path: filepath.Join(t.TempDir(), "parked.json")}
	if err := os.WriteFile(f.Path, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := f.Update(func(entries []Entry) []Entry { return nil })
	if err == nil {
		t.Fatal("updated a corrupt park file")
	}
	if data, _ := os.ReadFile(f.Path); string(data) != "{oops" {
		t.Fatalf("park file changed to %q", data)
	}
}

func TestSaveFailsWhenTheFolderIsAFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "claudes")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f := File{Path: filepath.Join(file, "parked.json")}
	if err := f.Save([]Entry{{SessionID: "a"}}); err == nil {
		t.Fatal("saved under a file")
	}
}

func TestALegacyFileIsReadInPlaceAndMovedByTheFirstWrite(t *testing.T) {
	dir := t.TempDir()
	f := File{Path: filepath.Join(dir, "new", "parked.json"), Legacy: filepath.Join(dir, "old", "parked.json")}
	old := File{Path: f.Legacy}
	if err := old.Save([]Entry{{SessionID: "a-1", Name: "a"}}); err != nil {
		t.Fatal(err)
	}

	entries, err := f.Load()
	if err != nil || len(entries) != 1 || entries[0].Name != "a" {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
	if _, err := os.Stat(f.Legacy); err != nil {
		t.Fatalf("a read moved the legacy file: %v", err)
	}
	if err := f.Update(func(e []Entry) []Entry { return e }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.Legacy); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("legacy file still there: %v", err)
	}
	if _, err := os.Stat(f.Path); err != nil {
		t.Fatal(err)
	}
}

func TestALegacyFileNeverReplacesTheCurrentOne(t *testing.T) {
	dir := t.TempDir()
	f := File{Path: filepath.Join(dir, "parked.json"), Legacy: filepath.Join(dir, "old.json")}
	if err := f.Save([]Entry{{SessionID: "current"}}); err != nil {
		t.Fatal(err)
	}
	if err := (File{Path: f.Legacy}).Save([]Entry{{SessionID: "legacy"}}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := f.Load(); len(entries) != 1 || entries[0].SessionID != "current" {
		t.Fatalf("entries %+v", entries)
	}
}

func TestEmptyingTheFileCannotBringTheLegacyOneBack(t *testing.T) {
	dir := t.TempDir()
	f := File{Path: filepath.Join(dir, "new", "parked.json"), Legacy: filepath.Join(dir, "old", "parked.json")}
	if err := f.Save([]Entry{{SessionID: "a-1", Name: "a"}}); err != nil {
		t.Fatal(err)
	}
	// A legacy file left behind, say by a remove that failed.
	if err := (File{Path: f.Legacy}).Save([]Entry{{SessionID: "b-1", Name: "b"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.Update(func([]Entry) []Entry { return nil }); err != nil {
		t.Fatal(err)
	}
	if entries, err := f.Load(); err != nil || len(entries) != 0 {
		t.Fatalf("entries %+v, err %v", entries, err)
	}
}
