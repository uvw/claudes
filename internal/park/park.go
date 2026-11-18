// Package park keeps the sessions `claudes pause` stopped, so `claudes resume`
// can bring them back, across a reboot if need be.
package park

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

const fileVersion = 1

// Entry is one paused session.
type Entry struct {
	SessionID string `json:"session_id"`
	JobID     string `json:"job_id,omitempty"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Kind      string `json:"kind"`
	WasBusy   bool   `json:"was_busy"`
	PausedAt  string `json:"paused_at"`
}

// FromSession records a session about to be paused. wasBusy is whether resume
// --ping prompts it, which the caller decides, since a lead whose teammates
// are at work counts though its own status may be idle.
func FromSession(s claude.Session, wasBusy bool, pausedAt string) Entry {
	return Entry{
		SessionID: s.SessionID,
		JobID:     s.JobID,
		Name:      s.Label(),
		Cwd:       s.Cwd,
		Kind:      s.Kind,
		WasBusy:   wasBusy,
		PausedAt:  pausedAt,
	}
}

// Interactive reports whether the session ran in a terminal.
func (e Entry) Interactive() bool { return e.Kind == claude.KindInteractive }

// ShortID is the first eight characters of the session id.
func (e Entry) ShortID() string { return claude.ShortID(e.SessionID) }

// ID is the id to show: the job id when there is one.
func (e Entry) ID() string { return cmp.Or(e.JobID, e.ShortID()) }

// Label is the name to show.
func (e Entry) Label() string { return shell.Printable(e.Name) }

// Dir is the directory the session ran in.
func (e Entry) Dir() string { return e.Cwd }

// Aliases are the exact strings that select this entry.
func (e Entry) Aliases() []string {
	return claude.NonEmpty(e.Name, e.JobID, e.SessionID, e.ShortID())
}

// ManualCommand is the shell command that brings the session back by hand.
func (e Entry) ManualCommand() string {
	return shell.ResumeCommand(e.SessionID, e.Cwd, e.Interactive())
}

// File is the park file on disk.
type File struct {
	Path string
	// Legacy is where an older claudes kept the file, if anywhere else. The
	// first Load moves it to Path.
	Legacy string
}

// DefaultFile is parked.json in claudes' state directory (paths.State).
func DefaultFile() (File, error) {
	dir, err := paths.State()
	if err != nil {
		return File{}, err
	}
	f := File{Path: filepath.Join(dir, "parked.json")}
	if legacy, err := paths.LegacyState(); err == nil && legacy != "" && legacy != dir {
		f.Legacy = filepath.Join(legacy, "parked.json")
	}
	return f, nil
}

// adopt moves the legacy file to Path when Path does not exist yet.
func (f File) adopt() error {
	if f.Legacy == "" {
		return nil
	}
	if _, err := os.Stat(f.Path); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	data, err := os.ReadFile(f.Legacy)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := files.WriteFileAll(f.Path, data, 0o644); err != nil {
		return err
	}
	return os.Remove(f.Legacy)
}

type payload struct {
	Version  int     `json:"version"`
	Sessions []Entry `json:"sessions"`
}

// Load reads the entries; a missing file has none.
func (f File) Load() ([]Entry, error) {
	path := f.Path
	if f.Legacy != "" && !exists(f.Path) && exists(f.Legacy) {
		path = f.Legacy
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", f.Path, err)
	}
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("%s is not a valid park file: %w", f.Path, err)
	}
	if p.Version > fileVersion {
		return nil, fmt.Errorf("%s was written by a newer claudes (version %d): update claudes", f.Path, p.Version)
	}
	return p.Sessions, nil
}

// Update changes the entries under a lock, so two claudes runs at once cannot
// lose each other's changes.
func (f File) Update(change func([]Entry) []Entry) error {
	lock := files.Lock{Path: f.Path + ".lock", Stale: 10 * time.Second, Attempts: 50, Delay: 10 * time.Millisecond}
	return lock.Do(func() error {
		if err := f.adopt(); err != nil {
			return fmt.Errorf("cannot move the park file from %s: %w", f.Legacy, err)
		}
		entries, err := f.Load()
		if err != nil {
			return err
		}
		return f.Save(change(entries))
	})
}

// Save writes the whole list atomically, or deletes the file when it is empty.
// An empty list deletes the legacy file too, so Load cannot bring back what it
// held.
func (f File) Save(entries []Entry) error {
	if len(entries) == 0 {
		for _, path := range claude.NonEmpty(f.Path, f.Legacy) {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload{Version: fileVersion, Sessions: entries}, "", "  ")
	if err != nil {
		return err
	}
	return files.WriteFile(f.Path, append(data, '\n'), 0o644)
}

// Merge keeps earlier entries; a session paused again replaces its old entry.
func Merge(existing, added []Entry) []Entry {
	out := make([]Entry, 0, len(existing)+len(added))
	index := map[string]int{}
	for _, e := range slices.Concat(existing, added) {
		if i, ok := index[e.SessionID]; ok {
			out[i] = e
			continue
		}
		index[e.SessionID] = len(out)
		out = append(out, e)
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
