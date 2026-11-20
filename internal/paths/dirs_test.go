package paths

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWithin(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	for dir, want := range map[string]bool{
		root:                              true,
		filepath.Join(root, "sub", "dir"): true,
		filepath.Dir(root):                false,
		root + "-other":                   false,
		filepath.Join(root, "..", "x"):    false,
		"":                                false,
	} {
		if got := Within(dir, root); got != want {
			t.Errorf("Within(%q, %q) = %v", dir, root, got)
		}
	}
	if rel, ok := RelWithin(root, filepath.Join(root, "a", "b")); !ok || rel != filepath.Join("a", "b") {
		t.Errorf("RelWithin: %q, %v", rel, ok)
	}
}

func TestRealResolvesLinksAndRefusesWhatIsNoDirectory(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "dir")
	file := filepath.Join(base, "file")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(dir, link); err == nil {
		if got, err := Real(link); err != nil || got != dir {
			t.Errorf("Real(link) = %q, %v", got, err)
		}
		if Resolved(link) != dir {
			t.Errorf("Resolved(link) = %q", Resolved(link))
		}
		if !Same(link, dir) {
			t.Error("a link and its directory are not the same")
		}
	}
	if _, err := Real(filepath.Join(base, "missing")); !errors.Is(err, ErrNoDir) {
		t.Errorf("a missing directory: %v", err)
	}
	if _, err := Real(file); err == nil {
		t.Error("a file is a directory")
	}
	if missing := filepath.Join(base, "missing"); Resolved(missing) != missing {
		t.Error("Resolved changed a missing directory")
	}
	if Same(dir, file) || !Same(dir, dir+string(filepath.Separator)) {
		t.Error("Same")
	}
}

func TestGoneAndIsDir(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing")
	if Gone(dir) || !Gone(missing) || Gone("") {
		t.Error("Gone")
	}
	if !IsDir(dir) || IsDir(missing) {
		t.Error("IsDir")
	}
}
