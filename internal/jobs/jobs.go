// Package jobs reads and writes the files Claude Code keeps for background
// sessions under ~/.claude/jobs. The Claude Code docs call that directory not a
// stable interface, so everything here may need to follow its changes.
package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/safefile"
)

// lockStale matches the stale time Claude Code passes to its lock for
// pins.json: an older lock is treated as abandoned.
const lockStale = 5 * time.Second

// Store is the jobs directory.
type Store struct {
	Dir string
	// LockAttempts and LockDelay bound the wait for the pins.json lock.
	LockAttempts int
	LockDelay    time.Duration
}

// DefaultStore is the jobs directory in Claude Code's directory (paths.ClaudeHome).
func DefaultStore() (Store, error) {
	base, err := paths.ClaudeHome()
	if err != nil {
		return Store{}, err
	}
	return Store{Dir: filepath.Join(base, "jobs"), LockAttempts: 10, LockDelay: 20 * time.Millisecond}, nil
}

func (s Store) pinsPath() string { return filepath.Join(s.Dir, "pins.json") }

// statePath is a job's state.json, or "" for an id that is not a job id, so
// a malformed id never names a path outside the jobs directory.
func (s Store) statePath(jobID string) string {
	if !ValidID(jobID) {
		return ""
	}
	return filepath.Join(s.Dir, jobID, "state.json")
}

var validID = regexp.MustCompile(`^[0-9a-f]{8}$`)

// ValidID reports whether id has the form of a job id: eight hex digits.
func ValidID(id string) bool { return validID.MatchString(id) }

// Exists reports whether the jobs directory is there. A missing one under a
// claude agents that lists background sessions means the wrong directory.
func (s Store) Exists() bool {
	info, err := os.Stat(s.Dir)
	return err == nil && info.IsDir()
}

// Pins returns the job ids pinned in agent view (Ctrl+T), in agent view's order.
func (s Store) Pins() ([]string, error) {
	data, err := os.ReadFile(s.pinsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, fmt.Errorf("%s is not a list of job ids: %w", s.pinsPath(), err)
	}
	return ids, nil
}

// PinSet returns the pinned job ids as a set.
func (s Store) PinSet() (map[string]bool, error) {
	ids, err := s.Pins()
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, err
}

// SetPinned pins or unpins job ids the way agent view does, and returns the
// ids whose state changed.
func (s Store) SetPinned(ids []string, pinned bool) ([]string, error) {
	var changed []string
	err := s.withLock(func() error {
		current, err := s.Pins()
		if err != nil {
			return err
		}
		next := slices.Clone(current)
		for _, id := range ids {
			has := slices.Contains(next, id)
			switch {
			case pinned && !has:
				next = append(next, id)
				changed = append(changed, id)
			case !pinned && has:
				next = slices.DeleteFunc(next, func(v string) bool { return v == id })
				changed = append(changed, id)
			}
		}
		if len(changed) == 0 {
			return nil
		}
		if err := writePins(s.pinsPath(), next); err != nil {
			return err
		}
		if !pinned {
			// Agent view drops a session's place in the pinned list when it unpins.
			for _, id := range changed {
				if !ValidID(id) {
					continue
				}
				err := os.Remove(filepath.Join(s.Dir, id, "order"))
				if err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
		}
		return nil
	})
	return changed, err
}

// writePins writes the list the way Claude Code does: a JSON array indented by
// two spaces, replaced atomically.
func writePins(path string, ids []string) error {
	if ids == nil {
		ids = []string{}
	}
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	return safefile.WriteFile(path, data, 0o644)
}

// withLock holds the same lock Claude Code takes for pins.json: a directory
// named pins.json.lock.
func (s Store) withLock(fn func() error) error {
	lock := safefile.Lock{Path: s.pinsPath() + ".lock", Stale: lockStale, Attempts: s.LockAttempts, Delay: s.LockDelay}
	return lock.Do(fn)
}

// LastActive is when a background session's state.json last changed, or zero
// when it is unknown.
func (s Store) LastActive(jobID string) time.Time {
	if jobID == "" {
		return time.Time{}
	}
	info, err := os.Stat(s.statePath(jobID))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// Has reports whether the session has a background job.
func (s Store) Has(jobID, sessionID string) bool {
	data, err := os.ReadFile(s.statePath(jobID))
	if err != nil {
		return false
	}
	var state struct {
		SessionID string `json:"sessionId"`
	}
	return json.Unmarshal(data, &state) == nil && state.SessionID == sessionID
}

// Folder is the directory a background session's job runs in, from its
// state.json, or "" when the job is unknown or belongs to another session.
// claude --bg --resume wakes an existing job there, wherever it is run from.
func (s Store) Folder(jobID, sessionID string) string {
	data, err := os.ReadFile(s.statePath(jobID))
	if err != nil {
		return ""
	}
	var state struct {
		SessionID string `json:"sessionId"`
		Cwd       string `json:"cwd"`
	}
	if json.Unmarshal(data, &state) != nil || state.SessionID != sessionID {
		return ""
	}
	return state.Cwd
}

// Options are the command-line options Claude Code saved for a background
// session's job, such as --model, and passes again each time it wakes the
// job. ok is false when the job is unknown or belongs to another session.
func (s Store) Options(jobID, sessionID string) (options []string, ok bool) {
	data, err := os.ReadFile(s.statePath(jobID))
	if err != nil {
		return nil, false
	}
	var state struct {
		SessionID    string   `json:"sessionId"`
		RespawnFlags []string `json:"respawnFlags"`
	}
	if json.Unmarshal(data, &state) != nil || state.SessionID != sessionID {
		return nil, false
	}
	return state.RespawnFlags, true
}

// Version is the Claude Code version a background session last ran, read from
// its state.json, or "" when it is unknown.
func (s Store) Version(jobID string) string {
	if jobID == "" {
		return ""
	}
	data, err := os.ReadFile(s.statePath(jobID))
	if err != nil {
		return ""
	}
	var state struct {
		CLIVersion string `json:"cliVersion"`
	}
	if json.Unmarshal(data, &state) != nil {
		return ""
	}
	return state.CLIVersion
}
