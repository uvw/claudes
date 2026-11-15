package transcripts

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/shell"
)

// Event is one thing that happened in a session, as claudes logs prints it: a
// prompt, a reply, a tool call, or the end of a turn.
type Event struct {
	Time time.Time
	// Who is you, claude, tool, turn, or team, for a message between the
	// members of an agent team.
	Who  string
	Text string
	// Raw is the transcript record the event came from.
	Raw json.RawMessage
}

type eventRecord struct {
	Type       string          `json:"type"`
	Subtype    string          `json:"subtype"`
	IsMeta     bool            `json:"isMeta"`
	Timestamp  time.Time       `json:"timestamp"`
	DurationMs int64           `json:"durationMs"`
	Message    json.RawMessage `json:"message"`
}

type message struct {
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// Events turns whole transcript lines into events. Records that are not a
// prompt, a reply, a tool call, or a turn's end, and lines that do not parse,
// are skipped.
func Events(data []byte) []Event {
	var out []Event
	for line := range bytes.Lines(data) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var r eventRecord
		if json.Unmarshal(line, &r) != nil || r.IsMeta {
			continue
		}
		raw := json.RawMessage(bytes.Clone(line))
		add := func(who, text string) {
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, Event{Time: r.Timestamp, Who: who, Text: text, Raw: raw})
			}
		}
		switch r.Type {
		case "user":
			for _, b := range blocks(r.Message) {
				if b.Type != "text" {
					continue
				}
				if messages := teammateMessages(b.Text); len(messages) > 0 {
					for _, m := range messages {
						add("team", m)
					}
					continue
				}
				if !claudeNote.MatchString(b.Text) {
					add("you", b.Text)
				}
			}
		case "assistant":
			for _, b := range blocks(r.Message) {
				switch b.Type {
				case "text":
					add("claude", b.Text)
				case "tool_use":
					add("tool", toolLine(b))
				}
			}
		case "system":
			if r.Subtype == "turn_duration" && r.DurationMs > 0 {
				add("turn", fmt.Sprintf("done in %s", (time.Duration(r.DurationMs)*time.Millisecond).Round(100*time.Millisecond)))
			}
		}
	}
	return out
}

// claudeNote matches the tagged text Claude Code adds in a user record, such
// as a slash command or a reminder, which the user did not type.
var claudeNote = regexp.MustCompile(`^<(command-|local-command-|system-reminder|bash-|user-prompt-submit-hook|task-notification|user-memory-input)`)

var teammateMessage = regexp.MustCompile(`(?s)<teammate-message teammate_id="([^"]*)"[^>]*>\s*(.*?)\s*(?:</teammate-message>|\z)`)

// teammateMessages turns the messages an agent team's members send each
// other into one line each, such as "tester: idle: Task 2 is complete". The
// lead gets them wrapped in a note about trusting peers, which is dropped.
func teammateMessages(text string) []string {
	var out []string
	for _, m := range teammateMessage.FindAllStringSubmatch(text, -1) {
		from, body := m[1], m[2]
		var note struct {
			Type    string `json:"type"`
			Result  string `json:"result"`
			Summary string `json:"summary"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
		}
		// The JSON can be followed by more text, so only its first value is read.
		if json.NewDecoder(strings.NewReader(body)).Decode(&note) == nil && note.Type != "" {
			kind := strings.TrimSuffix(note.Type, "_notification")
			detail := cmp.Or(note.Result, note.Summary, note.Message, note.Reason)
			out = append(out, strings.TrimSpace(from+": "+kind+": "+firstLine(detail)))
			continue
		}
		out = append(out, from+": "+firstLine(body))
	}
	return out
}

// blocks is a message's content: a plain string, or a list of blocks.
func blocks(msg json.RawMessage) []block {
	var m message
	if json.Unmarshal(msg, &m) != nil || len(m.Content) == 0 {
		return nil
	}
	var text string
	if json.Unmarshal(m.Content, &text) == nil {
		return []block{{Type: "text", Text: text}}
	}
	var bs []block
	_ = json.Unmarshal(m.Content, &bs)
	return bs
}

// toolLine names a tool call and its main argument, such as
// "Bash: go test ./..." or "Edit: internal/cli/ls.go".
func toolLine(b block) string {
	var in map[string]any
	_ = json.Unmarshal(b.Input, &in)
	for _, key := range []string{"command", "file_path", "path", "pattern", "url", "query", "description", "prompt"} {
		if v, ok := in[key].(string); ok && v != "" {
			return b.Name + ": " + firstLine(v)
		}
	}
	return b.Name
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

// Teammate returns the transcript of the named teammate of the team a
// session leads, from the folder beside the lead's transcript.
func (st Store) Teammate(sessionID, cwd, name string) (string, error) {
	lead, err := st.Find(sessionID, cwd)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(strings.TrimSuffix(lead, ".jsonl"), "subagents")
	var newest string
	var newestTime time.Time
	var names []string
	files, _ := filepath.Glob(filepath.Join(dir, "agent-a*.jsonl"))
	for _, f := range files {
		base := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(f), "agent-a"), ".jsonl")
		i := strings.LastIndexByte(base, '-')
		if i <= 0 {
			continue // a subagent, not a teammate
		}
		if !isTeammate(strings.TrimSuffix(f, ".jsonl") + ".meta.json") {
			continue // a named subagent, not a teammate
		}
		names = append(names, shell.Printable(base[:i]))
		if base[:i] != name {
			continue
		}
		if info, err := os.Stat(f); err == nil && info.ModTime().After(newestTime) {
			newest, newestTime = f, info.ModTime()
		}
	}
	if newest == "" {
		slices.Sort(names)
		return "", fmt.Errorf("no teammate %q in this session's team; teammates: %s", name, cmp.Or(strings.Join(slices.Compact(names), ", "), "none"))
	}
	return newest, nil
}

// isTeammate reports whether a subagent's meta file names a team.
func isTeammate(meta string) bool {
	data, err := os.ReadFile(meta)
	if err != nil {
		return false
	}
	var m struct {
		TeamName string `json:"teamName"`
	}
	return json.Unmarshal(data, &m) == nil && m.TeamName != ""
}

// Find returns the transcript of the session with this id: in the project
// folder for cwd when it is there, or else wherever it lives.
func (st Store) Find(sessionID, cwd string) (string, error) {
	name := sessionID + ".jsonl"
	if cwd != "" {
		p := filepath.Join(st.Dir, Folder(cwd), name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(st.Dir, "*", name))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no transcript for session %s under %s: %w", sessionID, st.Dir, fs.ErrNotExist)
	}
	// The newest copy, when session mv left more than one.
	newest, newestTime := "", time.Time{}
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.ModTime().After(newestTime) {
			newest, newestTime = m, info.ModTime()
		}
	}
	return cmp.Or(newest, matches[0]), nil
}
