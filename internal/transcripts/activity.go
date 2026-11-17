package transcripts

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// ActivityLimit is the largest transcript claudes counts the activity of when
// it reads many at once. A session with more in it than that is not trivial,
// and reading only the smaller ones keeps a full read of every transcript
// down to about a second.
const ActivityLimit = 2 << 20

// Activity is what a session did, counted from a full read of its transcript.
type Activity struct {
	// Prompts are the messages the user typed, images included. Slash
	// commands, their output, interruptions, and the text Claude Code adds,
	// such as reminders and compaction summaries, are not prompts.
	Prompts int `json:"prompts"`
	// Commands are the slash commands and ! shell commands the user ran.
	Commands  int `json:"commands"`
	ToolCalls int `json:"toolCalls"`
	// Edits are the tool calls that change a file.
	Edits int `json:"edits"`
	// FirstPrompt is the first prompt, cut to 200 characters.
	FirstPrompt string `json:"firstPrompt"`
	// Teammate is a session an agent team's lead started: its first message
	// came from the lead, not the user.
	Teammate bool `json:"teammate"`
}

// editTools are the tools that change files.
var editTools = map[string]bool{"Edit": true, "MultiEdit": true, "Write": true, "NotebookEdit": true}

type activityRecord struct {
	Type             string          `json:"type"`
	IsMeta           bool            `json:"isMeta"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Message          json.RawMessage `json:"message"`
}

// ReadActivity reads the transcript at path in full and counts what happened
// in it. Lines that do not parse are skipped.
func ReadActivity(path string) (Activity, error) {
	f, err := os.Open(path)
	if err != nil {
		return Activity{}, err
	}
	defer f.Close()
	var a Activity
	spoke := false
	rd := bufio.NewReader(f)
	for {
		line, err := rd.ReadBytes('\n')
		if len(line) > 0 {
			a.take(line, &spoke)
		}
		if err == io.EOF {
			return a, nil
		}
		if err != nil {
			return Activity{}, err
		}
	}
}

// take counts one transcript line. spoke is whether anyone has written to the
// session yet, the user or a team's lead.
func (a *Activity) take(line []byte, spoke *bool) {
	var r activityRecord
	if json.Unmarshal(line, &r) != nil || r.IsMeta || r.IsCompactSummary {
		return
	}
	switch r.Type {
	case "assistant":
		for _, b := range blocks(r.Message) {
			if b.Type == "tool_use" {
				a.ToolCalls++
				if editTools[b.Name] {
					a.Edits++
				}
			}
		}
	case "user":
		bs := blocks(r.Message)
		var text strings.Builder
		image := false
		for _, b := range bs {
			switch b.Type {
			case "tool_result":
				return
			case "text":
				text.WriteString(b.Text)
			case "image":
				image = true
			}
		}
		typed := strings.TrimSpace(text.String())
		switch {
		case strings.HasPrefix(typed, "<command-"), strings.HasPrefix(typed, "<bash-input"):
			a.Commands++
			*spoke = true
		case strings.HasPrefix(typed, "<teammate-message"):
			a.Teammate = a.Teammate || !*spoke
			*spoke = true
		case claudeNote.MatchString(typed), strings.HasPrefix(typed, "[Request interrupted"):
		case typed != "" || image:
			a.Prompts++
			*spoke = true
			if a.Prompts == 1 {
				a.FirstPrompt = cut(typed, 200)
			}
		}
	}
}

// cut is s with its runs of white space made single spaces, cut to at most
// limit characters.
func cut(s string, limit int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return string(r)
}
