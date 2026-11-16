package transcripts

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/uvw/claudes/internal/claude"
)

// Disk is the space one session takes, in bytes of file content.
type Disk struct {
	Transcript int64 `json:"transcript"`
	// Sidecar is the folder of subagent transcripts and tool results.
	Sidecar int64 `json:"sidecar"`
	// Copies are the older copies session mv left, with their sidecars.
	Copies int64 `json:"copies"`
	// FileHistory is Claude Code's backups of the files the session edited.
	FileHistory int64 `json:"fileHistory"`
	Total       int64 `json:"total"`
}

// Add adds o's space to d's.
func (d *Disk) Add(o Disk) {
	d.Transcript += o.Transcript
	d.Sidecar += o.Sidecar
	d.Copies += o.Copies
	d.FileHistory += o.FileHistory
	d.Total += o.Total
}

// FileHistory is the folder of Claude Code's backups of the files a session
// edited, or "" when the store does not know Claude Code's folder.
func (st Store) FileHistory(sessionID string) string {
	if st.Home == "" || !claude.ValidSessionID(sessionID) {
		return ""
	}
	return filepath.Join(st.Home, "file-history", sessionID)
}

// Usage measures the space s takes.
func (st Store) Usage(s Session) Disk {
	d := Disk{Transcript: fileSize(s.Path), Sidecar: treeSize(Sidecar(s))}
	for _, c := range s.Copies {
		d.Copies += fileSize(c) + treeSize(strings.TrimSuffix(c, ".jsonl"))
	}
	if dir := st.FileHistory(s.SessionID); dir != "" {
		d.FileHistory = treeSize(dir)
	}
	d.Total = d.Transcript + d.Sidecar + d.Copies + d.FileHistory
	return d
}

// Measure measures every session in list, in parallel. The result is in
// list's order.
func (st Store) Measure(list []Session) []Disk {
	out := make([]Disk, len(list))
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(runtime.NumCPU(), 8) {
		wg.Go(func() {
			for i := range next {
				out[i] = st.Usage(list[i])
			}
		})
	}
	for i := range list {
		next <- i
	}
	close(next)
	wg.Wait()
	return out
}

// Leftover is a session's folder whose transcript is gone.
type Leftover struct {
	Path      string `json:"path"`
	SessionID string `json:"sessionId"`
	// Kind is "sidecar" or "file-history".
	Kind string `json:"kind"`
	Size int64  `json:"size"`
	// Modified is when anything in the folder last changed.
	Modified time.Time `json:"modified"`
}

// Leftovers finds the sidecar folders with no transcript beside them, and the
// file-history folders of sessions list does not hold. Only folders named by
// a valid session id count, so a project's memory folder, or anything else
// another tool keeps there, never does.
func (st Store) Leftovers(list []Session) ([]Leftover, error) {
	known := map[string]bool{}
	for _, s := range list {
		known[s.SessionID] = true
	}
	var out []Leftover
	dirs, err := filepath.Glob(filepath.Join(st.Dir, "*", "*"))
	if err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		id := filepath.Base(dir)
		if !claude.ValidSessionID(id) || !isDir(dir) {
			continue
		}
		if _, err := os.Lstat(dir + ".jsonl"); err == nil {
			continue
		}
		size, modified := treeStat(dir)
		out = append(out, Leftover{Path: dir, SessionID: id, Kind: "sidecar", Size: size, Modified: modified})
	}
	if st.Home != "" {
		entries, err := os.ReadDir(filepath.Join(st.Home, "file-history"))
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() || !claude.ValidSessionID(e.Name()) || known[e.Name()] {
				continue
			}
			dir := st.FileHistory(e.Name())
			size, modified := treeStat(dir)
			out = append(out, Leftover{Path: dir, SessionID: e.Name(), Kind: "file-history", Size: size, Modified: modified})
		}
	}
	slices.SortFunc(out, func(a, b Leftover) int { return strings.Compare(a.Path, b.Path) })
	return out, nil
}

func isDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func fileSize(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

// treeSize adds up the regular files under dir. It never follows a link, and a
// dir that does not exist is empty.
func treeSize(dir string) int64 {
	n, _ := treeStat(dir)
	return n
}

// treeStat is treeSize, and when anything under dir last changed.
func treeStat(dir string) (int64, time.Time) {
	var n int64
	var newest time.Time
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if d.Type().IsRegular() {
			n += info.Size()
		}
		return nil
	})
	return n, newest
}

// CopyFiles are an older copy's transcript and, when there is one, its
// sidecar folder.
func CopyFiles(copyPath string) []string {
	out := []string{copyPath}
	if sidecar := strings.TrimSuffix(copyPath, ".jsonl"); isDir(sidecar) {
		out = append(out, sidecar)
	}
	return out
}

// SessionFiles is everything Claude Code keeps for s that exists: the
// transcript, its sidecar folder, every older copy with its sidecar, its file
// history, its session-env folder, and its debug log.
func (st Store) SessionFiles(s Session) []string {
	out := Files(s)
	for _, c := range s.Copies {
		out = append(out, CopyFiles(c)...)
	}
	if st.Home != "" && claude.ValidSessionID(s.SessionID) {
		for _, p := range []string{
			st.FileHistory(s.SessionID),
			filepath.Join(st.Home, "session-env", s.SessionID),
			filepath.Join(st.Home, "debug", s.SessionID+".txt"),
		} {
			if _, err := os.Lstat(p); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}
