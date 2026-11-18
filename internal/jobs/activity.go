package jobs

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"strings"
)

// Activity is what a background session's state.json says it is doing, as
// agent view shows it. Each field is nil when the file lacks it or gives it in
// a shape claudes does not know, so a change in Claude Code blanks a field
// rather than failing.
type Activity struct {
	// Subagents, Teammates, and Shells count the items in flight: subagents,
	// agent team teammates, and background shells.
	Subagents, Teammates, Shells *int
	// Tokens is agent view's running total of the tokens the session used.
	Tokens *int64
	// PRs counts the pull requests the session opened.
	PRs *int
	// Doing is agent view's one-line status of what the session is doing.
	Doing *string
}

// Activity reads a background session's state.json. An unknown job, or a file
// that does not parse, gives an Activity with every field nil.
func (s Store) Activity(jobID string) Activity {
	var a Activity
	if jobID == "" {
		return a
	}
	data, err := os.ReadFile(s.statePath(jobID))
	if err != nil {
		return a
	}
	var state struct {
		Fan      json.RawMessage `json:"fan"`
		InFlight json.RawMessage `json:"inFlight"`
		Tokens   json.RawMessage `json:"tokens"`
		Children json.RawMessage `json:"children"`
		Detail   json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(data, &state) != nil {
		return a
	}
	a.Subagents, a.Teammates, a.Shells = fanCounts(state.Fan, state.InFlight)
	a.Tokens = tokens(state.Tokens)
	a.PRs = prs(state.Children)
	var detail string
	if json.Unmarshal(state.Detail, &detail) == nil && strings.TrimSpace(detail) != "" {
		a.Doing = &detail
	}
	return a
}

// fanCounts counts the items in fan that still run. Claude Code leaves fan out
// when nothing it lists is in flight, so a missing fan counts as none only
// when inFlight says no tasks run, or only scheduled ones.
func fanCounts(fan, inFlight json.RawMessage) (subagents, teammates, shells *int) {
	if isAbsent(fan) {
		var f struct {
			Tasks *int     `json:"tasks"`
			Kinds []string `json:"kinds"`
		}
		if json.Unmarshal(inFlight, &f) != nil || f.Tasks == nil {
			return nil, nil, nil
		}
		for _, k := range f.Kinds {
			if k != "session_cron" {
				return nil, nil, nil
			}
		}
		if *f.Tasks != 0 && len(f.Kinds) == 0 {
			return nil, nil, nil
		}
		return ptr(0), ptr(0), ptr(0)
	}
	var items []json.RawMessage
	if json.Unmarshal(fan, &items) != nil {
		return nil, nil, nil
	}
	var a, t, sh int
	for _, raw := range items {
		var item struct {
			ID     string  `json:"id"`
			Kind   string  `json:"kind"`
			DoneAt float64 `json:"doneAt"`
		}
		// doneAt is 0 while an item runs.
		if json.Unmarshal(raw, &item) != nil || item.DoneAt != 0 {
			continue
		}
		switch fanKind(item.Kind, item.ID) {
		case fanSubagent:
			a++
		case fanTeammate:
			t++
		case fanShell:
			sh++
		}
	}
	return &a, &t, &sh
}

type inFlightKind int

const (
	fanOther inFlightKind = iota
	fanSubagent
	fanTeammate
	fanShell
)

// fanKind sorts an item of fan. Subagents and teammates both have kind agent;
// Claude Code starts a teammate's task id with t, and a subagent's with a.
func fanKind(kind, id string) inFlightKind {
	switch {
	case kind == "shell":
		return fanShell
	case kind == "agent" && strings.HasPrefix(id, "t"):
		return fanTeammate
	case kind == "agent":
		return fanSubagent
	}
	return fanOther
}

func tokens(raw json.RawMessage) *int64 {
	var n float64
	if json.Unmarshal(raw, &n) != nil || isAbsent(raw) || n < 0 || n != math.Trunc(n) || n > math.MaxInt64 {
		return nil
	}
	v := int64(n)
	return &v
}

// prs counts the pull requests in children, which is null before the session
// opens any.
func prs(raw json.RawMessage) *int {
	var children []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &children) != nil {
		return nil
	}
	n := 0
	for _, c := range children {
		var child struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(c, &child) == nil && child.Kind == "pr" {
			n++
		}
	}
	return &n
}

// isAbsent reports whether a field was left out or is null.
func isAbsent(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func ptr[T any](v T) *T { return &v }
