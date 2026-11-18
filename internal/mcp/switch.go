package mcp

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/shell"
)

// The results of one server in claudes mcp on, off, and use.
const (
	Done      = "done"
	Already   = "already"
	Planned   = "planned"
	Queued    = "queued"
	Refused   = "refused"
	Failed    = "error"
	TimedOut  = "timed out"
	Unchanged = "unchanged"
)

// The ways claudes mcp on, off, and use reach a directory's servers: through
// the session's claudes mod, by restarting the session, or by writing
// ~/.claude.json alone, for --in.
const (
	ViaMod     = "mod"
	ViaRestart = "restart"
	ViaFile    = "file"
)

// Change is one server that claudes mcp on, off, or use acts on.
type Change struct {
	Name string `json:"name"`
	// From is on or off, as disabledMcpServers has it before, and To is the
	// state asked for.
	From   string `json:"from"`
	To     string `json:"to"`
	Result string `json:"result"`
	Note   string `json:"note"`
}

// Pending reports whether nothing has happened to c yet.
func (c Change) Pending() bool { return c.Result == "" }

// Switch is what claudes mcp on, off, and use print with -o json or yaml.
type Switch struct {
	Command string `json:"command"`
	Dir     string `json:"dir"`
	// ID and Name are the session's, or "" with --in.
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Via     string   `json:"via"`
	DryRun  bool     `json:"dryRun"`
	Servers []Change `json:"servers"`
}

// Plan lists a change for each server in want, which maps a server's name to
// the state it should end in. A server that disabled already puts in that
// state is marked Already.
func (s *Switch) Plan(want map[string]string, disabled []string) {
	for _, name := range slices.Sorted(maps.Keys(want)) {
		c := Change{Name: name, From: StateIn(disabled, name), To: want[name]}
		if c.From == c.To {
			c.Result, c.Note = Already, "already "+c.To
		}
		s.Servers = append(s.Servers, c)
	}
}

// Pending reports whether any server is still to change.
func (s *Switch) Pending() bool { return slices.ContainsFunc(s.Servers, Change.Pending) }

// Mark gives every server still to change result and note.
func (s *Switch) Mark(result, note string) {
	for i := range s.Servers {
		if s.Servers[i].Pending() {
			s.Servers[i].Result, s.Servers[i].Note = result, note
		}
	}
}

// Apply is what disabledMcpServers becomes with the servers still to change.
func (s *Switch) Apply(current []string) []string {
	out := slices.Clone(current)
	for _, c := range s.Servers {
		if c.Result == Already {
			continue
		}
		switch {
		case c.To == Off && !slices.Contains(out, c.Name):
			out = append(out, c.Name)
		case c.To == On:
			out = slices.DeleteFunc(out, func(n string) bool { return n == c.Name })
		}
	}
	return out
}

// Verify settles each server for which acted returns true. It is Done when
// disabled puts it in the state asked for, and Unchanged, with note, when not.
func (s *Switch) Verify(disabled []string, acted func(Change) bool, note string) {
	for i, c := range s.Servers {
		if !acted(c) {
			continue
		}
		if StateIn(disabled, c.Name) == c.To {
			s.Servers[i].Result, s.Servers[i].Note = Done, ""
		} else {
			s.Servers[i].Result, s.Servers[i].Note = Unchanged, note
		}
	}
}

// Failed reports whether a server did not end as asked.
func (s *Switch) Failed() bool {
	return slices.ContainsFunc(s.Servers, func(c Change) bool {
		switch c.Result {
		case Refused, Failed, TimedOut, Unchanged:
			return true
		}
		return false
	})
}

// StateIn is a server's state as a disabledMcpServers list has it.
func StateIn(disabled []string, name string) string {
	if slices.Contains(disabled, name) {
		return Off
	}
	return On
}

// Verb is the /mcp subcommand that turns a server to state.
func Verb(state string) string {
	if state == On {
		return "enable"
	}
	return "disable"
}

// Want maps each of names to state.
func Want(names []string, state string) map[string]string {
	want := make(map[string]string, len(names))
	for _, n := range names {
		want[n] = state
	}
	return want
}

// Match picks the names each pattern matches: a glob over whole names, or one
// exact name. A pattern that matches nothing is an error.
func Match(patterns, names []string) ([]string, error) {
	var out []string
	for _, p := range patterns {
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("%q is not a valid pattern: %w", p, err)
		}
		found := false
		for _, n := range names {
			if ok, _ := path.Match(p, n); ok || p == n {
				found = true
				if !slices.Contains(out, n) {
					out = append(out, n)
				}
			}
		}
		if !found {
			return nil, noServer(p, names)
		}
	}
	return out, nil
}

func noServer(pattern string, names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("no MCP server matches %q: the directory has none", pattern)
	}
	shown := make([]string, len(names))
	for i, n := range names {
		shown[i] = shell.Printable(n)
	}
	return fmt.Errorf("no MCP server matches %q. Servers: %s", pattern, strings.Join(shown, ", "))
}
