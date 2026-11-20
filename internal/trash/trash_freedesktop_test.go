//go:build unix && !darwin

package trash

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPutBackRemovesTheTrashInfo(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "Trash")
	src := filepath.Join(root, "claude", "debug", "s.txt")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("d"), 0o644); err != nil {
		t.Fatal(err)
	}
	to, err := toFreedesktop(src, bin, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := putBack(File{From: src, To: to}); err != nil {
		t.Fatal(err)
	}
	if !exists(src) || exists(filepath.Join(bin, "info", "s.txt.trashinfo")) {
		t.Fatalf("back %v, info left %v", exists(src), exists(filepath.Join(bin, "info", "s.txt.trashinfo")))
	}
}
