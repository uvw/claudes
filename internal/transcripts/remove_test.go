package transcripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSidecarPartsAreTheNonEmptyOnesWithTheirSizes(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	s := Session{SessionID: sid, Path: filepath.Join(root, "-a", sid+".jsonl")}
	write(t, filepath.Join(Sidecar(s), "tool-results", "r.txt"), at, strings.Repeat("a", 100))
	write(t, filepath.Join(Sidecar(s), "subagents", "agent-1.jsonl"), at, strings.Repeat("b", 1000))
	if err := os.MkdirAll(filepath.Join(Sidecar(s), "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	parts := SidecarParts(s, false)
	if len(parts) != 1 || parts[0].What != "saved outputs" || parts[0].Size != 100 {
		t.Fatalf("without subagents: %+v", parts)
	}
	parts = SidecarParts(s, true)
	if len(parts) != 2 || parts[1].Path != filepath.Join(Sidecar(s), "subagents") || parts[1].Size != 1000 {
		t.Fatalf("with subagents: %+v", parts)
	}
	if err := os.Remove(filepath.Join(Sidecar(s), "tool-results", "r.txt")); err != nil {
		t.Fatal(err)
	}
	if parts := SidecarParts(s, false); len(parts) != 0 {
		t.Fatalf("an empty tool-results is a part: %+v", parts)
	}
}

func TestRemoveUsesTheMoverItIsGiven(t *testing.T) {
	root := t.TempDir()
	st := Store{Dir: filepath.Join(root, "projects"), Home: root}
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	path := filepath.Join(st.Dir, "-a", sid+".jsonl")
	write(t, path, at, "{}\n")
	var moved []string
	err := st.Remove(sid, []string{path, filepath.Join(st.Dir, "-a", sid)}, func(p string) error {
		moved = append(moved, p)
		return nil
	})
	if err != nil || len(moved) != 1 || moved[0] != path {
		t.Fatalf("moved %q, err %v: a path that is gone is skipped, the rest moved", moved, err)
	}
}
