//go:build windows

package trash

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveTakesAFileAndADirectoryToTheRecycleBin(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "s.jsonl")
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "s")
	if err := os.MkdirAll(filepath.Join(dir, "tool-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool-results", "r.txt"), []byte("result"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, dir} {
		if err := Move(path); err != nil {
			t.Fatalf("move %s: %v", path, err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s is still there: %v", path, err)
		}
	}
}
