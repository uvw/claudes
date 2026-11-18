package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEditRunsVisualThenEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in editor is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "edited")
	editor := filepath.Join(dir, "editor")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n"
	if err := os.WriteFile(editor, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", editor+" --wait")
	t.Setenv("EDITOR", "")
	if err := Edit("a.yaml"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", editor)
	if err := Edit("b.yaml"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if want := "--wait a.yaml\nb.yaml\n"; string(got) != want {
		t.Fatalf("the editor ran with %q, want %q", got, want)
	}
}

func TestEditWithNoEditorSaysHowToFixIt(t *testing.T) {
	t.Setenv("VISUAL", filepath.Join(t.TempDir(), "no-such-editor"))
	err := Edit("config.yaml")
	if err == nil || !strings.Contains(err.Error(), "set VISUAL or EDITOR") {
		t.Fatalf("err %v", err)
	}
}
