package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriteFileReplacesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	for _, data := range []string{"[]", `["a"]`} {
		if err := WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(path); string(got) != data {
			t.Fatalf("got %q, want %q", got, data)
		}
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestWriteFileWhileAReaderHasTheFileOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pins.json")
	if err := WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
		f.Close()
	}()
	if err := WriteFile(path, []byte(`["a"]`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newLock(t *testing.T) Lock {
	return Lock{Path: filepath.Join(t.TempDir(), "pins.json.lock"), Stale: time.Second, Attempts: 200, Delay: time.Millisecond}
}

func TestLockExcludesOtherHolders(t *testing.T) {
	l := newLock(t)
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				l.Do(func() error {
					if inside.Add(1) > 1 {
						overlaps.Add(1)
					}
					time.Sleep(100 * time.Microsecond)
					inside.Add(-1)
					return nil
				})
			}
		})
	}
	wg.Wait()
	if overlaps.Load() > 0 {
		t.Fatalf("%d overlapping holders", overlaps.Load())
	}
}

func TestLockBreaksAStaleLockOnce(t *testing.T) {
	l := newLock(t)
	if err := os.Mkdir(l.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(l.Path, old, old); err != nil {
		t.Fatal(err)
	}
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			l.Do(func() error {
				if inside.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(time.Millisecond)
				inside.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()
	if overlaps.Load() > 0 {
		t.Fatalf("%d overlapping holders", overlaps.Load())
	}
	if _, err := os.Stat(l.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestLockLeavesALockItNoLongerOwns(t *testing.T) {
	l := newLock(t)
	err := l.Do(func() error {
		// Another process broke this lock as stale and made its own.
		os.Remove(l.Path)
		time.Sleep(20 * time.Millisecond)
		return os.Mkdir(l.Path, 0o755)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path); err != nil {
		t.Fatalf("removed the other process's lock: %v", err)
	}
}

func TestLockGivesUp(t *testing.T) {
	l := newLock(t)
	l.Attempts = 3
	if err := os.Mkdir(l.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := l.Do(func() error { return nil }); err == nil {
		t.Fatal("took a held lock")
	}
}

func TestWriteFileIntoAMissingFolderFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "pins.json")
	if err := WriteFile(path, []byte("[]"), 0o644); err == nil {
		t.Fatal("wrote into a folder that does not exist")
	}
}

func TestWriteFileOverAFolderFailsAndCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pins.json")
	if err := os.MkdirAll(filepath.Join(path, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(path, []byte("[]"), 0o644); err == nil {
		t.Fatal("replaced a folder")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestLockReturnsTheErrorAndReleases(t *testing.T) {
	l := newLock(t)
	want := errors.New("boom")
	if err := l.Do(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("err %v", err)
	}
	if _, err := os.Stat(l.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestLockFailsWhenItsFolderCannotBeMade(t *testing.T) {
	file := filepath.Join(t.TempDir(), "jobs")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l := Lock{Path: filepath.Join(file, "pins.json.lock"), Attempts: 1}
	ran := false
	if err := l.Do(func() error { ran = true; return nil }); err == nil || ran {
		t.Fatalf("err %v, ran %v", err, ran)
	}
}

func TestLockTakesALockItBreaksOnTheLastAttempt(t *testing.T) {
	l := newLock(t)
	l.Attempts, l.Stale = 1, 50*time.Millisecond
	if err := os.Mkdir(l.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(l.Path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := l.Do(func() error { return nil }); err != nil {
		t.Fatalf("broke the lock and still gave up: %v", err)
	}
}

func TestLockStaysFreshWhileItsHolderIsSlow(t *testing.T) {
	l := newLock(t)
	l.Stale = 200 * time.Millisecond
	other := l
	other.Attempts, other.Delay = 1, 0
	err := l.Do(func() error {
		time.Sleep(3 * l.Stale)
		if other.Do(func() error { return nil }) == nil {
			return errors.New("another process broke a lock that was in use")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the refreshed lock was not released: %v", err)
	}
}
