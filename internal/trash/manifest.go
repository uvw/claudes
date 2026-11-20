package trash

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

// manifestVersion is the version of trashed.json this claudes writes. It
// refuses a newer one rather than rewrite it and lose what it holds.
const manifestVersion = 1

// File is a file or directory claudes moved to the trash: where it was, where
// the trash put it, and the bytes it holds.
type File struct {
	From string `json:"from"`
	To   string `json:"to"`
	Size int64  `json:"size"`
}

// Record is what one command moved to the trash for one session.
type Record struct {
	// SessionID is the session's id, or the short id that names a team
	// directory.
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	Files     []File `json:"files"`
	// Moves are the moves.json entries the command dropped along with the
	// files, as transcripts.Store.ForgetMoves returned them. Restore hands
	// them back once every file is back.
	Moves json.RawMessage `json:"moves,omitempty"`
}

// Run is what one claudes command moved to the trash.
type Run struct {
	At      time.Time `json:"at"`
	Command string    `json:"command"`
	Records []Record  `json:"records"`
}

type manifest struct {
	Version int   `json:"version"`
	Runs    []Run `json:"runs"`
}

// Manifest is trashed.json, claudes' record of where each file it moved to
// the trash came from, so Restore can put it back. The system trash keeps no
// such record on macOS, and none Windows gives back. A Manifest with no Path
// records nothing.
type Manifest struct {
	Path string
}

// DefaultManifest is trashed.json in claudes' state directory (paths.State).
func DefaultManifest() (Manifest, error) {
	dir, err := paths.State()
	if err != nil {
		return Manifest{}, err
	}
	return Manifest{Path: filepath.Join(dir, "trashed.json")}, nil
}

// Add records r, what command moved to the trash for one session in the run
// that started at at. A file the trash did not say where it put, as on
// Windows, is left out.
func (m Manifest) Add(at time.Time, command string, r Record) error {
	r.Files = slices.DeleteFunc(slices.Clone(r.Files), func(f File) bool { return f.To == "" })
	if m.Path == "" || len(r.Files) == 0 {
		return nil
	}
	for i := range r.Files {
		r.Files[i].Size = treeSize(r.Files[i].To)
	}
	return m.update(func(d *manifest) error {
		i := slices.IndexFunc(d.Runs, func(run Run) bool { return run.At.Equal(at) && run.Command == command })
		if i < 0 {
			d.Runs = append(d.Runs, Run{At: at, Command: command})
			i = len(d.Runs) - 1
		}
		d.Runs[i].Records = append(d.Runs[i].Records, r)
		return nil
	})
}

// Entry is everything claudes moved to the trash for one session, by every
// command that did.
type Entry struct {
	ShortID   string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Cwd       string `json:"cwd"`
	// Trashed is when the latest of its files went to the trash.
	Trashed time.Time `json:"trashed"`
	Size    int64     `json:"size"`
	// Missing counts the files no longer in the trash, which Restore refuses.
	Missing int     `json:"missing"`
	Files   []Moved `json:"files"`
}

// Moved is one file of an entry, with the command that moved it and when.
type Moved struct {
	File
	Command string    `json:"command"`
	Trashed time.Time `json:"trashed"`
	InTrash bool      `json:"inTrash"`
}

// ID is the session's short id.
func (e Entry) ID() string { return e.ShortID }

// Label is the name to show, or the short id.
func (e Entry) Label() string { return cmp.Or(shell.Printable(e.Name), e.ID()) }

// Dir is the directory the session ran in.
func (e Entry) Dir() string { return e.Cwd }

// Aliases are the exact strings that select the entry.
func (e Entry) Aliases() []string {
	var out []string
	for _, a := range []string{e.Name, e.SessionID, e.ID()} {
		if a != "" {
			out = append(out, a)
		}
	}
	return out
}

// Holds reports whether one of the entry's files came from path.
func (e Entry) Holds(path string) bool {
	return slices.ContainsFunc(e.Files, func(m Moved) bool { return m.From == path })
}

// shortID is the first eight characters of a session id: a team directory's
// name holds only those, so they tie its record to its session's.
func shortID(id string) string { return id[:min(8, len(id))] }

// Entries are the sessions with a file still in the trash, the latest
// trashed first. It reads without the lock, since the manifest is replaced
// whole.
func (m Manifest) Entries() ([]Entry, error) {
	d, err := m.load()
	if err != nil {
		return nil, err
	}
	byID := map[string]int{}
	var out []Entry
	for _, run := range d.Runs {
		for _, r := range run.Records {
			i, seen := byID[shortID(r.SessionID)]
			if !seen {
				i = len(out)
				byID[shortID(r.SessionID)] = i
				out = append(out, Entry{})
			}
			e := &out[i]
			e.SessionID, e.ShortID = longer(e.SessionID, r.SessionID), shortID(r.SessionID)
			if !run.At.Before(e.Trashed) {
				e.Name, e.Cwd, e.Trashed = r.Name, r.Cwd, run.At
			}
			for _, f := range r.Files {
				in := exists(f.To)
				e.Files = append(e.Files, Moved{File: f, Command: run.Command, Trashed: run.At, InTrash: in})
				if in {
					e.Size += f.Size
				} else {
					e.Missing++
				}
			}
		}
	}
	out = slices.DeleteFunc(out, func(e Entry) bool { return e.Missing == len(e.Files) })
	slices.SortStableFunc(out, func(a, b Entry) int { return b.Trashed.Compare(a.Trashed) })
	return out, nil
}

// longer is whichever of a and b is longer: a whole session id rather than a
// short one.
func longer(a, b string) string {
	if len(b) > len(a) {
		return b
	}
	return a
}

// Plan is every file Restore would put back for the sessions whose short ids
// are ids, in the order it would, and every reason it would refuse.
func (m Manifest) Plan(ids []string) ([]File, []error) {
	d, err := m.load()
	if err != nil {
		return nil, []error{err}
	}
	return plan(d, ids)
}

// plan orders the files of ids so a directory comes back before anything
// that goes inside it, as when --sidecars trimmed a session that session gc
// later removed whole.
func plan(d manifest, ids []string) ([]File, []error) {
	var out []File
	var problems []error
	found := map[string]bool{}
	from := map[string]string{}
	for _, run := range slices.Backward(d.Runs) {
		for _, r := range run.Records {
			if !slices.Contains(ids, shortID(r.SessionID)) {
				continue
			}
			found[shortID(r.SessionID)] = true
			for _, f := range r.Files {
				if other, twice := from[f.From]; twice {
					problems = append(problems, fmt.Errorf("%s went to the trash twice, as %s and %s, so claudes cannot tell which to put back", f.From, other, f.To))
					continue
				}
				from[f.From] = f.To
				switch {
				case !exists(f.To):
					problems = append(problems, fmt.Errorf("%s is no longer in the trash, where it was %s", f.From, f.To))
				case exists(f.From):
					problems = append(problems, fmt.Errorf("%s exists, and claudes never replaces it", f.From))
				}
				out = append(out, f)
			}
		}
	}
	for _, id := range ids {
		if !found[id] {
			problems = append(problems, fmt.Errorf("claudes has no record of %s in the trash", id))
		}
	}
	slices.SortStableFunc(out, func(a, b File) int {
		return cmp.Compare(strings.Count(a.From, string(filepath.Separator)), strings.Count(b.From, string(filepath.Separator)))
	})
	return out, problems
}

// Blocked is a restore refused before it moved anything, with every reason.
type Blocked struct {
	Problems []error
}

func (b *Blocked) Error() string {
	lines := make([]string, len(b.Problems))
	for i, p := range b.Problems {
		lines[i] = p.Error()
	}
	return strings.Join(lines, "\n")
}

// Restored is what a restore put back, and what it could not.
type Restored struct {
	Back   []File
	Failed []Failure
	// Moves are the Record.Moves of each record whose every file is back.
	Moves []json.RawMessage
}

// Failure is a file that stayed in the trash, and why.
type Failure struct {
	File
	Err error
}

// Restore puts back every file of the sessions whose short ids are ids,
// holding the manifest's lock throughout, so two runs cannot both move a
// file. It refuses, as a *Blocked, and moves nothing, when Plan finds a
// problem. A file it cannot move stays in the trash and in the manifest, so a
// later Restore tries it again.
func (m Manifest) Restore(ids []string) (Restored, error) {
	var res Restored
	err := m.update(func(d *manifest) error {
		todo, problems := plan(*d, ids)
		if len(problems) > 0 {
			return &Blocked{Problems: problems}
		}
		back := map[File]bool{}
		for _, f := range todo {
			if err := putBack(f); err != nil {
				res.Failed = append(res.Failed, Failure{File: f, Err: err})
				continue
			}
			back[f] = true
			res.Back = append(res.Back, f)
		}
		for i := range d.Runs {
			var kept []Record
			for _, r := range d.Runs[i].Records {
				r.Files = slices.DeleteFunc(r.Files, func(f File) bool { return back[f] })
				if len(r.Files) > 0 {
					kept = append(kept, r)
				} else if len(r.Moves) > 0 {
					res.Moves = append(res.Moves, r.Moves)
				}
			}
			d.Runs[i].Records = kept
		}
		return nil
	})
	return res, err
}

// putBack moves f from the trash to where it came from, never over anything
// there, and creates the directories above it that are gone.
func putBack(f File) error {
	if err := os.MkdirAll(filepath.Dir(f.From), 0o700); err != nil {
		return err
	}
	if err := files.RenameNoReplace(f.To, f.From); err != nil {
		return err
	}
	forget(f.To)
	return nil
}

// update changes the manifest under its lock. A manifest claudes cannot read
// is kept, rather than replaced by one that would lose every record.
func (m Manifest) update(change func(*manifest) error) error {
	lock := files.Lock{Path: m.Path + ".lock", Stale: 10 * time.Second, Attempts: 50, Delay: 10 * time.Millisecond}
	return lock.Do(func() error {
		d, err := m.load()
		if err != nil {
			return err
		}
		if err := change(&d); err != nil {
			return err
		}
		return m.save(d)
	})
}

func (m Manifest) load() (manifest, error) {
	d := manifest{Version: manifestVersion}
	if m.Path == "" {
		return d, nil
	}
	data, err := os.ReadFile(m.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, fmt.Errorf("%s is not a valid trash record: %w", m.Path, err)
	}
	if d.Version > manifestVersion {
		return d, fmt.Errorf("%s was written by a newer claudes (version %d): update claudes", m.Path, d.Version)
	}
	return d, nil
}

// save writes d atomically, without the records whose every file has left the
// trash, as when the trash was emptied. A manifest with no records left is
// deleted.
func (m Manifest) save(d manifest) error {
	for i := range d.Runs {
		d.Runs[i].Records = slices.DeleteFunc(d.Runs[i].Records, func(r Record) bool {
			return !slices.ContainsFunc(r.Files, func(f File) bool { return exists(f.To) })
		})
	}
	d.Runs = slices.DeleteFunc(d.Runs, func(run Run) bool { return len(run.Records) == 0 })
	if len(d.Runs) == 0 {
		if err := os.Remove(m.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	d.Version = manifestVersion
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return files.WriteFileAll(m.Path, append(data, '\n'), 0o600)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// treeSize adds up the regular files at and under path, without following a
// link.
func treeSize(path string) int64 {
	var n int64
	// The walk never fails: it skips what it cannot read.
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}
