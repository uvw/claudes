package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMoveIntoNumbersANameTheTrashHolds(t *testing.T) {
	root := t.TempDir()
	trash := filepath.Join(root, "Trash")
	for i := range 2 {
		src := filepath.Join(root, "work", "a.jsonl")
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte{byte('0' + i)}, 0o644); err != nil {
			t.Fatal(err)
		}
		dest, err := moveInto(src, trash)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"a.jsonl", "a.jsonl 2"}[i]
		if filepath.Base(dest) != want {
			t.Fatalf("move %d went to %s, want %s", i, dest, want)
		}
		if _, err := os.Lstat(src); !os.IsNotExist(err) {
			t.Fatalf("the original is still there: %v", err)
		}
	}
}

func TestFreedesktopRecordsWhereItCameFrom(t *testing.T) {
	root := t.TempDir()
	trash := filepath.Join(root, "Trash")
	src := filepath.Join(root, "my work", "s")
	if err := os.MkdirAll(filepath.Join(src, "tool-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 16, 9, 30, 0, 0, time.Local)
	if err := toFreedesktop(src, trash, at); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(trash, "files", "s", "tool-results")); err != nil {
		t.Fatalf("not in files/: %v", err)
	}
	info, err := os.ReadFile(filepath.Join(trash, "info", "s.trashinfo"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(info), "Path="+escapePath(src)+"\n") || !strings.Contains(string(info), "my%20work") ||
		!strings.Contains(string(info), "DeletionDate=2026-11-16T09:30:00\n") {
		t.Fatalf("trashinfo:\n%s", info)
	}

	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := toFreedesktop(src, trash, at); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(trash, "info", "s.2.trashinfo")); err != nil {
		t.Fatalf("second copy not numbered: %v", err)
	}
}

func TestFreedesktopLeavesNoInfoWhenTheMoveFails(t *testing.T) {
	trash := filepath.Join(t.TempDir(), "Trash")
	if err := toFreedesktop(filepath.Join(t.TempDir(), "missing"), trash, time.Now()); err == nil {
		t.Fatal("moved a file that does not exist")
	}
	if entries, _ := os.ReadDir(filepath.Join(trash, "info")); len(entries) != 0 {
		t.Fatalf("info left behind: %v", entries)
	}
}
