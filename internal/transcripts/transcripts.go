// Package transcripts reads the session transcripts Claude Code keeps under
// ~/.claude/projects, one directory per working directory, and copies one to
// another directory. Their format is not a stable Claude Code interface: records
// claudes does not recognize are skipped, though one keeps a session from
// counting as empty or trivial.
package transcripts

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/files"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

// Store is the projects directory.
type Store struct {
	Dir string
	// Home is Claude Code's directory, which holds each session's file history,
	// or "" to leave it out.
	Home string
	// Moves is claudes' own record of the directories session mv copied
	// sessions to, or "" for none. A copy's records still name the directory it
	// came from until it is resumed, and a project directory's name cannot be
	// turned back into the directory.
	Moves string
}

// Session is what claudes reads from one transcript.
type Session struct {
	SessionID string `json:"sessionId"`
	// Name is the name given with /rename, or agent view's name, or the title
	// Claude Code wrote for it, in that order.
	Name string `json:"name"`
	// NameSource is where Name came from, or "" when it has none.
	NameSource string    `json:"nameSource"`
	Cwd        string    `json:"cwd"`
	Branch     string    `json:"branch"`
	Started    time.Time `json:"started"`
	LastActive time.Time `json:"lastActive"`
	LastPrompt string    `json:"lastPrompt"`
	Version    string    `json:"version"`
	// Path is the transcript. A session moved with claudes session mv has a
	// copy in each directory; Path is the newest, and Copies lists the rest.
	Path   string   `json:"path"`
	Copies []string `json:"copies"`
	Size   int64    `json:"size"`
	// Empty is a session that never got a prompt: its transcript, short
	// enough to read whole, holds only record types claudes knows to be
	// bookkeeping.
	Empty bool `json:"empty"`
	// fits is whether the transcript's own records place it in its project
	// directory, or Claude Code relocated it, so no recorded move applies.
	fits    bool
	options []string
}

// The values of Session.NameSource: where a session's name came from.
const (
	// NameCustom is a name given with /rename.
	NameCustom = "custom"
	// NameAgent is agent view's name for a background session.
	NameAgent = "agent"
	// NameAI is the title Claude Code wrote for the session.
	NameAI = "ai"
)

// JobOptions are the options of the background job session mv removed when
// it moved the session, for the job claude --bg --resume starts in its place.
func (s Session) JobOptions() []string { return s.options }

// ID is the id to show: the first eight characters of the session id, as
// agent view shows it.
func (s Session) ID() string { return claude.ShortID(s.SessionID) }

// Label is the name to show: the name, or the short id when it has none.
func (s Session) Label() string { return cmp.Or(shell.Printable(s.Name), s.ID()) }

// Aliases are the exact strings that select the session.
func (s Session) Aliases() []string { return claude.NonEmpty(s.Name, s.SessionID, s.ID()) }

// Folder is the project directory that holds a working directory's transcripts:
// every character that is not an ASCII letter or digit becomes '-'. Claude
// Code counts UTF-16 code units, so a character outside the Basic
// Multilingual Plane, such as an emoji, becomes two.
func Folder(cwd string) string {
	var b strings.Builder
	for _, r := range cwd {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case r > 0xFFFF:
			b.WriteString("--")
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// maxFolder is the longest directory name Claude Code uses as is. It shortens a
// longer one and adds a hash claudes does not reproduce.
const maxFolder = 200

// List reads every session in the store, newest activity first. A transcript
// that cannot be read is left out.
func (st Store) List() ([]Session, error) {
	// Glob ignores I/O errors, so one on the directory itself is caught here.
	if _, err := os.Stat(st.Dir); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	files, err := filepath.Glob(filepath.Join(st.Dir, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	read := make([]*Session, len(files))
	var wg sync.WaitGroup
	next := make(chan int)
	for range min(runtime.NumCPU(), 8) {
		wg.Go(func() {
			for i := range next {
				if s, err := summarize(files[i]); err == nil && claude.ValidSessionID(s.SessionID) {
					read[i] = &s
				}
			}
		})
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()

	moves, _ := st.loadMoves()
	moved := func(s *Session) bool { _, ok := moves[moveKey(s.Path, s.SessionID)]; return ok }
	byID := map[string]int{}
	var out []Session
	for _, s := range read {
		if s == nil {
			continue
		}
		i, seen := byID[s.SessionID]
		if !seen {
			byID[s.SessionID] = len(out)
			out = append(out, *s)
			continue
		}
		kept := &out[i]
		// On a tie, as right after session mv, the copy wins.
		if s.LastActive.After(kept.LastActive) || s.LastActive.Equal(kept.LastActive) && moved(s) {
			s.Copies = append(kept.Copies, kept.Path)
			*kept = *s
		} else {
			kept.Copies = append(kept.Copies, s.Path)
		}
	}
	for i := range out {
		m, ok := moves[moveKey(out[i].Path, out[i].SessionID)]
		if ok && !out[i].fits {
			out[i].Cwd = m.Dir
		}
		out[i].options = m.Options
		// Every key is always present in -o json and -o yaml, a list as [].
		if out[i].Copies == nil {
			out[i].Copies = []string{}
		}
	}
	slices.SortStableFunc(out, func(a, b Session) int { return b.LastActive.Compare(a.LastActive) })
	return out, nil
}

// moveKey names a copy in the moves file: its project directory and session id.
// The directory is in lower case, since Windows and macOS put a copy into an
// existing directory whose name differs only in case.
func moveKey(path, sessionID string) string {
	return strings.ToLower(filepath.Base(filepath.Dir(path))) + "/" + sessionID
}

// move is the directory a copy belongs to, and the options of the background
// job session mv removed for it. A move with no options is written as the
// directory alone, as claudes versions without options wrote it. Those
// versions reject the whole file once any entry holds options.
type move struct {
	Dir     string   `json:"dir"`
	Options []string `json:"options,omitempty"`
}

func (m move) MarshalJSON() ([]byte, error) {
	if len(m.Options) == 0 {
		return json.Marshal(m.Dir)
	}
	type plain move
	return json.Marshal(plain(m))
}

func (m *move) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &m.Dir); err == nil {
		return nil
	}
	type plain move
	return json.Unmarshal(data, (*plain)(m))
}

// loadMoves reads the moves file. A missing one has no moves.
func (st Store) loadMoves() (map[string]move, error) {
	moves := map[string]move{}
	if st.Moves == "" {
		return moves, nil
	}
	data, err := os.ReadFile(st.Moves)
	if errors.Is(err, fs.ErrNotExist) {
		return moves, nil
	}
	if err != nil {
		return moves, err
	}
	if err := json.Unmarshal(data, &moves); err != nil {
		return map[string]move{}, fmt.Errorf("%s is not a valid moves file: %w", st.Moves, err)
	}
	return moves, nil
}

// recordMove notes that the copy at path belongs to dir.
func (st Store) recordMove(path, sessionID string, m move) error {
	return st.updateMoves(func(moves map[string]move) bool {
		moves[moveKey(path, sessionID)] = m
		return true
	})
}

// ForgetMoves drops the record of each copy at paths, once those copies are
// gone.
func (st Store) ForgetMoves(sessionID string, paths ...string) error {
	return st.updateMoves(func(moves map[string]move) bool {
		changed := false
		for _, path := range paths {
			key := moveKey(path, sessionID)
			_, ok := moves[key]
			delete(moves, key)
			changed = changed || ok
		}
		return changed
	})
}

// updateMoves changes the moves file under a lock, so two claudes runs at
// once cannot lose each other's moves. change reports whether it changed
// anything. A moves file claudes cannot read is kept, rather than replaced by
// one that would lose every other move.
func (st Store) updateMoves(change func(map[string]move) bool) error {
	if st.Moves == "" {
		return nil
	}
	lock := files.Lock{Path: st.Moves + ".lock", Stale: 10 * time.Second, Attempts: 50, Delay: 10 * time.Millisecond}
	return lock.Do(func() error {
		moves, err := st.loadMoves()
		if err != nil {
			return err
		}
		if !change(moves) {
			return nil
		}
		data, err := json.MarshalIndent(moves, "", "  ")
		if err != nil {
			return err
		}
		return files.WriteFileAll(st.Moves, append(data, '\n'), 0o600)
	})
}

// headSize and tailSize bound what Read takes from each end of a transcript.
// Claude Code writes the session's title and last prompt again every few
// turns, so the tail almost always holds them.
const (
	headSize = 64 << 10
	tailSize = 256 << 10
)

// record is the part of a transcript line claudes reads.
type record struct {
	Type        string `json:"type"`
	SessionID   string `json:"sessionId"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Timestamp   string `json:"timestamp"`
	Version     string `json:"version"`
	CustomTitle string `json:"customTitle"`
	AgentName   string `json:"agentName"`
	AITitle     string `json:"aiTitle"`
	LastPrompt  string `json:"lastPrompt"`
	// RelocatedCwd is where Claude Code moved the session to.
	RelocatedCwd string `json:"relocatedCwd"`
	// Message is whether the record carries a message, which a user, an
	// assistant, or an API request record does.
	Message present `json:"message"`
}

// present is whether a JSON field is there, without keeping its value.
type present bool

func (p *present) UnmarshalJSON([]byte) error { *p = true; return nil }

// bookkeeping are the record types Claude Code writes about a session rather
// than as its conversation, which a session that never got a prompt can hold.
var bookkeeping = setOf("agent-name", "agent-setting", "ai-title", "artifact-autoreact-ledger",
	"artifact-comment-monitor", "atis-latch", "attachment", "continued-in", "cost-state", "custom-title",
	"file-history-delta", "file-history-snapshot", "frame-link", "last-prompt", "mode", "permission-mode",
	"pr-link", "queue-operation", "relocated", "session-owner", "system", "tools-used", "worktree-state")

// known are the record types claudes recognizes: bookkeeping, the
// conversation, and the API requests made for it.
var known = setOf(append(slices.Collect(maps.Keys(bookkeeping)),
	"user", "assistant", "api-request", "api-request-blob", "api-request-shape")...)

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// empty reports whether a transcript holds nothing but bookkeeping. It takes
// positive evidence: Claude Code adds and renames record types, so a type
// claudes does not know, a record with a message, or a line that does not
// parse could be conversation, and the transcript is not empty.
func empty(rs []record, clean bool) bool {
	if !clean {
		return false
	}
	for _, r := range rs {
		if bool(r.Message) || !bookkeeping[r.Type] {
			return false
		}
	}
	return true
}

// summarize reads one transcript from its first 64 KB and its last 256 KB.
func summarize(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Session{}, err
	}
	s := Session{Path: path, Size: info.Size(), LastActive: info.ModTime().UTC()}

	head := make([]byte, min(info.Size(), headSize))
	if _, err := io.ReadFull(f, head); err != nil {
		return Session{}, err
	}
	var tail []byte
	if info.Size() > headSize {
		start := max(headSize, info.Size()-tailSize)
		tail = make([]byte, info.Size()-start)
		if _, err := f.ReadAt(tail, start); err != nil && !errors.Is(err, io.EOF) {
			return Session{}, err
		}
	}
	var sum summary
	headRecords, clean := records(head, false)
	for _, r := range headRecords {
		sum.take(r)
	}
	tailRecords, _ := records(tail, true)
	for _, r := range tailRecords {
		sum.take(r)
	}
	if sum.cwd == "" && info.Size() > headSize {
		// Very long lines filled both ends, or one straddled them, so read
		// the whole file.
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Session{}, err
		}
		rd := bufio.NewReader(f)
		for {
			line, err := rd.ReadBytes('\n')
			rs, _ := records(line, false)
			for _, r := range rs {
				sum.take(r)
			}
			if err != nil {
				break
			}
		}
	}
	s.SessionID = cmp.Or(sum.sessionID, strings.TrimSuffix(filepath.Base(path), ".jsonl"))
	switch {
	case sum.customTitle != "":
		s.Name, s.NameSource = sum.customTitle, NameCustom
	case sum.agentName != "":
		s.Name, s.NameSource = sum.agentName, NameAgent
	case sum.aiTitle != "":
		s.Name, s.NameSource = sum.aiTitle, NameAI
	}
	project := filepath.Base(filepath.Dir(path))
	s.Cwd = sum.dir(project)
	s.fits = sum.relocated != "" || strings.EqualFold(Folder(s.Cwd), project)
	s.Branch, s.Version, s.LastPrompt, s.Started = sum.branch, sum.version, sum.lastPrompt, sum.started
	s.Empty = info.Size() <= headSize && empty(headRecords, clean)
	return s, nil
}

// summary collects what the records of one transcript say, read in order: the
// first session id and timestamp, and the newest of everything else.
type summary struct {
	sessionID, firstCwd, cwd, relocated string
	branch, version, lastPrompt         string
	customTitle, agentName, aiTitle     string
	started                             time.Time
}

func (m *summary) take(r record) {
	m.sessionID = cmp.Or(m.sessionID, r.SessionID)
	if m.started.IsZero() {
		m.started, _ = time.Parse(time.RFC3339Nano, r.Timestamp)
	}
	m.firstCwd = cmp.Or(m.firstCwd, r.Cwd)
	m.cwd = cmp.Or(r.Cwd, m.cwd)
	m.relocated = cmp.Or(r.RelocatedCwd, m.relocated)
	m.branch = cmp.Or(r.GitBranch, m.branch)
	m.version = cmp.Or(r.Version, m.version)
	m.lastPrompt = cmp.Or(r.LastPrompt, m.lastPrompt)
	m.customTitle = cmp.Or(r.CustomTitle, m.customTitle)
	m.agentName = cmp.Or(r.AgentName, m.agentName)
	m.aiTitle = cmp.Or(r.AITitle, m.aiTitle)
}

// directory is the session's directory: where Claude Code relocated it, or else the
// recorded directory that names the project directory it lives in, so a session
// that wandered into a subdirectory resumes where Claude Code looks for it.
func (m *summary) dir(project string) string {
	switch {
	case m.relocated != "":
		return m.relocated
	case Folder(m.firstCwd) == project:
		return m.firstCwd
	case Folder(m.cwd) == project:
		return m.cwd
	}
	return cmp.Or(m.firstCwd, m.cwd)
}

// records parses the JSON lines in data. With partial, the first line may be
// cut off, so it is skipped; a cut-off or unreadable line is skipped anyway,
// and clean is whether every line that is not blank parsed.
func records(data []byte, partial bool) (out []record, clean bool) {
	if partial {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	clean = true
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		var r record
		switch {
		case json.Unmarshal(line, &r) == nil:
			out = append(out, r)
		case len(bytes.TrimSpace(line)) > 0:
			clean = false
		}
	}
	return out, clean
}

// Copy copies s's transcript, and its directory of subagent transcripts and tool
// results when there is one, into the project directory for dir. It never moves
// or deletes: the original stays, so the session still resumes from its old
// directory. It returns the directory it copied into. options are kept as the
// copy's JobOptions.
func (st Store) Copy(s Session, dir string, options []string) (string, error) {
	name := Folder(dir)
	if len(name) > maxFolder {
		return "", fmt.Errorf("%s is too long: Claude Code shortens the names of such directories in a way claudes cannot match, so it would not find the copy", dir)
	}
	dest := filepath.Join(st.Dir, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	// The same directory can differ in case, or be reached through a link.
	if same(filepath.Dir(s.Path), dest) {
		return "", fmt.Errorf("%s already lives under %s", s.Label(), dir)
	}
	target := filepath.Join(dest, filepath.Base(s.Path))
	if _, err := os.Lstat(target); err == nil {
		return "", fmt.Errorf("%s already has a copy at %s, which may hold work the original lacks. Remove it first to replace it", s.Label(), target)
	}
	sidecar, sidecarTarget := Sidecar(s), filepath.Join(dest, filepath.Base(Sidecar(s)))
	if info, err := os.Stat(sidecar); err != nil || !info.IsDir() {
		sidecar = ""
	} else if _, err := os.Lstat(sidecarTarget); err == nil {
		return "", fmt.Errorf("%s already has a subagent directory at %s, with no transcript beside it. Remove it first to copy the session there", s.Label(), sidecarTarget)
	}
	if err := stage(s.Path, sidecar, target, sidecarTarget); err != nil {
		return "", err
	}
	if err := st.recordMove(target, s.SessionID, move{Dir: dir, Options: options}); err != nil {
		if sidecar != "" {
			os.RemoveAll(sidecarTarget)
		}
		os.Remove(target)
		return "", fmt.Errorf("removed the copy, since claudes cannot record where it belongs: %w", err)
	}
	return dest, nil
}

// stage copies a transcript, and its sidecar unless that is "", into a
// temporary directory beside target, then renames them into place, the
// sidecar first. So a failure leaves no part of a copy behind, and a re-run
// does not find one in its way.
func stage(transcript, sidecar, target, sidecarTarget string) error {
	tmp, err := os.MkdirTemp(filepath.Dir(target), ".claudes-mv-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	staged := filepath.Join(tmp, filepath.Base(target))
	if err := copyFile(transcript, staged); err != nil {
		return err
	}
	if sidecar != "" {
		stagedSidecar := filepath.Join(tmp, filepath.Base(sidecarTarget))
		if err := copyTree(sidecar, stagedSidecar); err != nil {
			return err
		}
		if err := files.RenameNoReplace(stagedSidecar, sidecarTarget); err != nil {
			return err
		}
	}
	if err := files.RenameNoReplace(staged, target); err != nil {
		if sidecar != "" {
			os.RemoveAll(sidecarTarget)
		}
		return err
	}
	return nil
}

// RemoveOriginal removes s's transcript and its directory of subagent
// transcripts and tool results with remove, once every file of the copy in
// dest matches the original byte for byte. remove moves a path to the system
// trash, or is os.RemoveAll to delete it for good.
func (st Store) RemoveOriginal(s Session, dest string, remove func(string) error) error {
	copied := filepath.Join(dest, filepath.Base(s.Path))
	if same(s.Path, copied) {
		return fmt.Errorf("%s is the original, not a copy", copied)
	}
	if err := sameContent(s.Path, copied); err != nil {
		return fmt.Errorf("kept the original, since the copy does not match it: %w", err)
	}
	sidecar := Sidecar(s)
	if err := sameTree(sidecar, filepath.Join(dest, filepath.Base(sidecar))); err != nil {
		return fmt.Errorf("kept the original, since the copy of its subagent directory does not match it: %w", err)
	}
	if isDir(sidecar) {
		if err := remove(sidecar); err != nil {
			return err
		}
	}
	return remove(s.Path)
}

// sameTree checks that every file under src has an identical copy under
// dest. A src that does not exist matches. Anything but a directory or a regular
// file, such as a link, is not copied, so it does not match.
func sameTree(src, dest string) error {
	if _, err := os.Lstat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file, so it was not copied", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		return sameContent(path, filepath.Join(dest, rel))
	})
}

func sameContent(a, b string) error {
	da, err := os.ReadFile(a)
	if err != nil {
		return err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return err
	}
	if !bytes.Equal(da, db) {
		return fmt.Errorf("%s and %s differ", a, b)
	}
	return nil
}

func same(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(ai, bi)
}

// Sidecar is the directory beside a transcript that holds its subagent
// transcripts and tool results.
func Sidecar(s Session) string { return strings.TrimSuffix(s.Path, ".jsonl") }

func copyTree(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

// copyFile copies src to dest atomically, and keeps src's modification time,
// which is when the session was last active.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if err := files.WriteFrom(dest, in, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dest, info.ModTime(), info.ModTime())
}

// Files are the transcript and, when there is one, its sidecar directory: what
// removing the session from its directory would take.
func Files(s Session) []string {
	out := []string{s.Path}
	if info, err := os.Stat(Sidecar(s)); err == nil && info.IsDir() {
		out = append(out, Sidecar(s))
	}
	return out
}

// DefaultStore is the projects directory in Claude Code's directory
// (paths.ClaudeHome).
func DefaultStore() (Store, error) {
	home, err := paths.ClaudeHome()
	if err != nil {
		return Store{}, err
	}
	st := Store{Dir: filepath.Join(home, "projects"), Home: home}
	if state, err := paths.State(); err == nil {
		st.Moves = filepath.Join(state, "moves.json")
	}
	return st, nil
}

// FolderNamesMatch reports whether Folder still names the project directories
// Claude Code writes: at least one of the ten most recently active sessions
// lives in the directory Folder gives for its working directory. Older sessions
// and moved copies can differ, so one match among the newest is enough. With
// no sessions there is nothing to contradict it.
func FolderNamesMatch(newestFirst []Session) bool {
	checked := 0
	for _, s := range newestFirst {
		if s.Cwd == "" {
			continue
		}
		if filepath.Base(filepath.Dir(s.Path)) == Folder(s.Cwd) {
			return true
		}
		if checked++; checked == 10 {
			return false
		}
	}
	return checked == 0
}
