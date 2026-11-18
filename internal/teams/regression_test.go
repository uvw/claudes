package teams

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// A junction inside a leftover directory must be removed as a link, never
// followed into the directory it points to.
func TestRemoveDoesNotFollowAJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows only")
	}
	root := t.TempDir()
	st := Store{Teams: filepath.Join(root, "teams"), Tasks: filepath.Join(root, "tasks")}
	outside := filepath.Join(root, "precious")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	team := filepath.Join(st.Teams, "session-aaaaaaaa")
	if err := os.MkdirAll(team, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(team, "link"), outside).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	os.Chtimes(team, old, old)
	os.Chtimes(filepath.Join(team, "link"), old, old)
	folders, err := st.Leftovers(map[string]bool{}, time.Now())
	if err != nil || len(folders) != 1 {
		t.Fatalf("leftovers %v, err %v", folders, err)
	}
	if err := st.Remove(folders[0], os.RemoveAll); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("Remove followed the junction: %v", err)
	}
	if _, err := os.Stat(team); err == nil {
		t.Fatal("team directory left")
	}
}

// A team directory that is itself a junction is skipped, not walked or removed.
func TestLeftoversSkipAJunctionedFolder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows only")
	}
	root := t.TempDir()
	st := Store{Teams: filepath.Join(root, "teams"), Tasks: filepath.Join(root, "tasks")}
	outside := filepath.Join(root, "precious")
	os.MkdirAll(outside, 0o755)
	os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("x"), 0o644)
	os.MkdirAll(st.Teams, 0o755)
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(st.Teams, "session-bbbbbbbb"), outside).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	folders, err := st.Leftovers(map[string]bool{}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 0 {
		t.Fatalf("a junction counts as a leftover: %+v", folders)
	}
}

// A config whose leadSessionId names another session with the same short id
// is not that session's team.
func TestLeadChecksTheFullID(t *testing.T) {
	root := t.TempDir()
	st := Store{Teams: filepath.Join(root, "teams")}
	dir := filepath.Join(st.Teams, "session-aaaaaaaa")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"name":"session-aaaaaaaa","leadSessionId":"aaaaaaaa-0000-0000-0000-000000000001","members":[{"name":"x","agentType":"general-purpose"}]}`), 0o644)
	if _, ok := st.Lead("aaaaaaaa-0000-0000-0000-000000000002"); ok {
		t.Fatal("another session's team")
	}
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{not json`), 0o644)
	if _, ok := st.Lead("aaaaaaaa-0000-0000-0000-000000000001"); ok {
		t.Fatal("bad json read as a team")
	}
	if got := (Store{}).Teammates("x"); len(got) != 0 {
		t.Fatal(got)
	}
}
