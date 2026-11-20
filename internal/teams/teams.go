// Package teams reads Claude Code's agent teams: the team configs under
// ~/.claude/teams and the shared task lists under ~/.claude/tasks. A team is
// named session-<lead's short id>, and while teammates run, its config lists
// them beside the lead. It also builds the rows claudes team ls shows, and
// finds the directories claudes gc --teams removes.
package teams

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/paths"
)

// leadType is the agent type of a team's lead in its member list.
const leadType = "team-lead"

// Store is the two directories Claude Code keeps teams in.
type Store struct {
	// Teams holds one directory per team, with its config.json.
	Teams string
	// Tasks holds one directory per session's task list, team or not.
	Tasks string
}

// DefaultStore is the teams and tasks directories in Claude Code's directory.
func DefaultStore() (Store, error) {
	home, err := paths.ClaudeHome()
	if err != nil {
		return Store{}, err
	}
	return Store{Teams: filepath.Join(home, "teams"), Tasks: filepath.Join(home, "tasks")}, nil
}

// Team is one team, as its config describes it.
type Team struct {
	Name          string   `json:"name"`
	LeadSessionID string   `json:"leadSessionId"`
	Members       []Member `json:"members"`
	CreatedAt     int64    `json:"createdAt"`
}

// Member is the lead or a teammate.
type Member struct {
	Name      string `json:"name"`
	AgentType string `json:"agentType"`
	Backend   string `json:"backendType"`
	Cwd       string `json:"cwd"`
}

// Teammates names the members other than the lead: the ones that run now,
// since Claude Code drops a teammate from the list when it shuts down.
func (t Team) Teammates() []string {
	names := []string{}
	for _, m := range t.Members {
		if m.AgentType != leadType {
			names = append(names, m.Name)
		}
	}
	return names
}

// Name is the team name for a lead session.
func Name(sessionID string) string { return "session-" + claude.ShortID(sessionID) }

// Lead reads the team the session leads, if it has one.
func (st Store) Lead(sessionID string) (Team, bool) {
	t, err := st.read(Name(sessionID))
	if err != nil || t.LeadSessionID != sessionID {
		return Team{}, false
	}
	return t, true
}

// Unknown stands for the teammates of a team whose config.json claudes cannot
// read, so the lead is still treated as one with teammates.
const Unknown = "unknown (its team config cannot be read)"

// Teammates names the teammates the session's team runs now, or none. A
// team config that exists but cannot be read gives Unknown.
func (st Store) Teammates(sessionID string) []string {
	t, err := st.read(Name(sessionID))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return []string{}
	case err != nil || t.Name == "" || t.LeadSessionID == "":
		return []string{Unknown}
	case t.LeadSessionID != sessionID:
		return []string{}
	}
	return t.Teammates()
}

// Unreadable lists the team directories whose config.json exists but does not
// parse, or lacks the team's name or lead.
func (st Store) Unreadable() ([]string, error) {
	entries, err := os.ReadDir(st.Teams)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var bad []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		t, err := st.read(e.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || t.Name == "" || t.LeadSessionID == "" {
			bad = append(bad, e.Name())
		}
	}
	return bad, nil
}

func (st Store) read(name string) (Team, error) {
	var t Team
	if st.Teams == "" {
		return t, fs.ErrNotExist
	}
	data, err := os.ReadFile(filepath.Join(st.Teams, name, "config.json"))
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("%s: %w", name, err)
	}
	return t, nil
}

// List reads every team. A team that cannot be read is left out.
func (st Store) List() ([]Team, error) {
	entries, err := os.ReadDir(st.Teams)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Team
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if t, err := st.read(e.Name()); err == nil && t.Name == e.Name() {
			out = append(out, t)
		}
	}
	return out, nil
}

// Task is one entry in a shared task list.
type Task struct {
	ID        string   `json:"id"`
	Subject   string   `json:"subject"`
	Status    string   `json:"status"`
	Owner     string   `json:"owner"`
	BlockedBy []string `json:"blockedBy"`
}

// TaskList reads the task list of a team or session, in id order.
func (st Store) TaskList(name string) ([]Task, error) {
	files, err := filepath.Glob(filepath.Join(st.Tasks, name, "*.json"))
	if err != nil {
		return nil, err
	}
	out := []Task{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var t Task
		if json.Unmarshal(data, &t) == nil && t.ID != "" {
			if t.BlockedBy == nil {
				t.BlockedBy = []string{}
			}
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b Task) int { return compareIDs(a.ID, b.ID) })
	return out, nil
}

func compareIDs(a, b string) int {
	ai, errA := strconv.Atoi(a)
	bi, errB := strconv.Atoi(b)
	if errA == nil && errB == nil {
		return ai - bi
	}
	return strings.Compare(a, b)
}

// Leftover is a team or task-list directory that gc can remove.
type Leftover struct {
	Path string
	// Name is the directory's name, session-<short id>.
	Name     string
	Modified time.Time
}

// ID is the short id of the session the directory belonged to.
func (l Leftover) ID() string { return strings.TrimPrefix(l.Name, "session-") }

// Label is what the directory is: a team or a task list.
func (l Leftover) Label() string {
	if filepath.Base(filepath.Dir(l.Path)) == "teams" {
		return "team"
	}
	return "task list"
}

// Dir is the directory itself.
func (l Leftover) Dir() string { return l.Path }

// Leftovers lists the team and task-list directories whose session is not in
// keep and that last changed before cutoff. keep holds short session ids.
func (st Store) Leftovers(keep map[string]bool, cutoff time.Time) ([]Leftover, error) {
	var out []Leftover
	for _, dir := range []string{st.Teams, st.Tasks} {
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			short, ok := strings.CutPrefix(e.Name(), "session-")
			if !e.IsDir() || !ok || keep[short] {
				continue
			}
			path := filepath.Join(dir, e.Name())
			modified, err := newest(path)
			if err != nil || !modified.Before(cutoff) {
				continue
			}
			out = append(out, Leftover{Path: path, Name: e.Name(), Modified: modified})
		}
	}
	return out, nil
}

// newest is when a file in the directory last changed, or the directory itself when
// it holds no files.
func newest(dir string) (time.Time, error) {
	var t time.Time
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
		return nil
	})
	if err == nil && t.IsZero() {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			return t, statErr
		}
		t = info.ModTime()
	}
	return t, err
}

// Remove removes a directory Leftovers returned with remove: a move to the
// system trash, or os.RemoveAll to delete it for good.
func (st Store) Remove(f Leftover, remove func(string) error) error {
	if filepath.Dir(f.Path) != st.Teams && filepath.Dir(f.Path) != st.Tasks {
		return fmt.Errorf("%s is not a team or task directory", f.Path)
	}
	return remove(f.Path)
}
