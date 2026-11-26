package transcripts

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// accountStores lays out three accounts: main, a profile that shares main's
// projects through a link as cswap does, and one with its own. It returns a
// Store over all three and the session ids it wrote: one in main's projects
// and one in the third account's.
func accountStores(t *testing.T) (st Store, shared, own string) {
	t.Helper()
	root := t.TempDir()
	main, linked, apart := filepath.Join(root, "main"), filepath.Join(root, "linked"), filepath.Join(root, "apart")
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	shared, own = sid, "0a1b2c3d-0000-4000-8000-000000000001"
	write(t, filepath.Join(main, "projects", "-a", shared+".jsonl"), at, line(t, map[string]any{"type": "user", "sessionId": shared, "cwd": "/a"}))
	write(t, filepath.Join(apart, "projects", "-b", own+".jsonl"), at, line(t, map[string]any{"type": "user", "sessionId": own, "cwd": "/b"}))
	if err := os.MkdirAll(linked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(main, "projects"), filepath.Join(linked, "projects")); err != nil {
		t.Skip("cannot make a link:", err)
	}
	for _, home := range []string{main, linked} {
		write(t, filepath.Join(home, "file-history", shared, "f@v1"), at, "abcde")
		write(t, filepath.Join(home, "session-env", shared, "env"), at, "x")
	}
	write(t, filepath.Join(linked, "file-history", "0a1b2c3d-0000-4000-8000-0000000000ff", "f@v1"), at, "old")
	st = Store{
		Dir: filepath.Join(main, "projects"), Home: main,
		Projects: []string{filepath.Join(linked, "projects"), filepath.Join(apart, "projects")},
		Homes:    []string{linked, apart},
	}
	return st, shared, own
}

func TestListReadsASharedProjectsDirectoryOnce(t *testing.T) {
	st, shared, own := accountStores(t)
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range list {
		ids = append(ids, s.SessionID)
		if len(s.Copies) != 0 {
			t.Errorf("%s has copies %v", s.SessionID, s.Copies)
		}
	}
	slices.Sort(ids)
	if want := []string{own, shared}; !slices.Equal(ids, want) {
		t.Fatalf("ids %v, want %v", ids, want)
	}
	if p, err := st.Find(own, "/b"); err != nil || filepath.Base(p) != own+".jsonl" {
		t.Fatalf("find %q, err %v", p, err)
	}
	if p, err := st.Find(own, ""); err != nil || filepath.Base(p) != own+".jsonl" {
		t.Fatalf("find without a directory %q, err %v", p, err)
	}
}

func TestASessionsFilesCountInEveryAccount(t *testing.T) {
	st, shared, _ := accountStores(t)
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(list, func(s Session) bool { return s.SessionID == shared })
	s := list[i]
	if d := st.Usage(s); d.FileHistory != 10 {
		t.Errorf("file history %d bytes, want 10 from both accounts", d.FileHistory)
	}
	if n := st.ExtraSize(s); n != 2 {
		t.Errorf("extra size %d, want 2", n)
	}
	files := st.SessionFiles(s)
	for _, home := range []string{st.Home, st.Homes[0]} {
		for _, want := range []string{filepath.Join(home, "file-history", shared), filepath.Join(home, "session-env", shared)} {
			if !slices.Contains(files, want) {
				t.Errorf("session files %q lack %s", files, want)
			}
		}
	}
	if err := st.Remove(shared, files, os.RemoveAll); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Homes[0], "file-history", shared)); !os.IsNotExist(err) {
		t.Fatalf("the linked account's file history is still there: %v", err)
	}
	left, err := st.Leftovers(nil)
	if err != nil || len(left) != 1 || left[0].Kind != "file-history" || filepath.Dir(filepath.Dir(left[0].Path)) != st.Homes[0] {
		t.Fatalf("leftovers %+v, err %v", left, err)
	}
}

func TestRemoveRefusesAPathOutsideEveryAccount(t *testing.T) {
	st, shared, _ := accountStores(t)
	outside := filepath.Join(filepath.Dir(st.Home), "elsewhere", shared)
	if err := st.Remove(shared, []string{outside}, os.RemoveAll); err == nil {
		t.Fatal("removed a path outside the accounts")
	}
}

func TestCopyStaysInTheSessionsOwnProjectsDirectory(t *testing.T) {
	st, _, own := accountStores(t)
	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	s := list[slices.IndexFunc(list, func(s Session) bool { return s.SessionID == own })]
	dest, err := st.Copy(s, "/c", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(st.Projects[1], Folder("/c")); dest != want {
		t.Fatalf("copied to %s, want %s", dest, want)
	}
}
