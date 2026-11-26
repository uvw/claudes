package transcripts

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SidecarPart is a directory of a session's sidecar that session gc can trim
// while keeping the session.
type SidecarPart struct {
	Path string
	// What is what the directory holds, such as "saved outputs".
	What string
	Size int64
}

// SidecarParts are the parts of s's sidecar that hold anything: the command
// outputs Claude Code saved under tool-results, and with withSubagents the
// subagent transcripts under subagents.
func SidecarParts(s Session, withSubagents bool) []SidecarPart {
	parts := []SidecarPart{{Path: "tool-results", What: "saved outputs"}}
	if withSubagents {
		parts = append(parts, SidecarPart{Path: "subagents", What: "subagents"})
	}
	var out []SidecarPart
	for _, p := range parts {
		p.Path = filepath.Join(Sidecar(s), p.Path)
		if !isDir(p.Path) {
			continue
		}
		if p.Size = treeSize(p.Path); p.Size > 0 {
			out = append(out, p)
		}
	}
	return out
}

// sessionExtras are the files SessionFiles holds beyond what Usage measures:
// a session's session-env directory and its debug log, in each Claude Code
// directory.
func (st Store) sessionExtras(sessionID string) []string {
	var out []string
	for _, home := range st.homes() {
		out = append(out, filepath.Join(home, "session-env", sessionID), filepath.Join(home, "debug", sessionID+".txt"))
	}
	return out
}

// ExtraSize is the space SessionFiles takes beyond s's Usage: its session-env
// directories and debug logs.
func (st Store) ExtraSize(s Session) int64 { return Size(st.sessionExtras(s.SessionID)) }

// Size adds up the regular files at and under paths.
func Size(paths []string) int64 {
	var n int64
	for _, p := range paths {
		n += treeSize(p)
	}
	return n
}

// under reports whether p is inside root, and not root itself.
func under(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// Changed is when the file at path last changed, or zero when it is gone.
func Changed(path string) time.Time {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// Remove removes each of paths, files of session sessionID, with remove: a move
// to the system trash, or os.RemoveAll to delete for good. It refuses a path
// outside the Claude Code directories and projects directories the store
// knows, or one whose name, or whose directory's name, does not carry the
// session id, so nothing else is ever touched. A path that is already gone is
// skipped.
func (st Store) Remove(sessionID string, paths []string, remove func(string) error) error {
	roots := append(st.homes(), st.projectDirs()...)
	for _, p := range paths {
		owned := strings.Contains(filepath.Base(p), sessionID) || filepath.Base(filepath.Dir(p)) == sessionID
		if !owned || !slices.ContainsFunc(roots, func(root string) bool { return under(root, p) }) {
			return fmt.Errorf("refused to remove %s, which is not a file of session %s under %s", p, sessionID, cmp.Or(st.Home, st.Dir))
		}
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := remove(p); err != nil {
			return err
		}
	}
	return nil
}
