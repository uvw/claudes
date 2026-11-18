package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/uvw/claudes/internal/safefile"
)

// ErrNoProject means ~/.claude.json has no entry for a directory, because no
// Claude Code session has run there.
var ErrNoProject = errors.New("~/.claude.json has no entry for this directory yet: start a Claude Code session there once")

// errChanged means ~/.claude.json kept changing while claudes tried to write it.
var errChanged = errors.New("~/.claude.json changed while claudes was writing it, three times in a row: try again")

// Disabled is the disabledMcpServers list of dir's entry in ~/.claude.json,
// the servers /mcp turned off there.
func (s Store) Disabled(dir string) ([]string, error) {
	var r reader
	cfg := r.claudeJSON(s.ConfigFile)
	if len(r.problems) > 0 {
		return nil, r.problems[0]
	}
	return stringsOf(cfg.project(dir)["disabledMcpServers"]), nil
}

// ProjectKey is the key of dir's entry in ~/.claude.json, or "" when it has
// none. Two directories with one key share their MCP settings.
func (s Store) ProjectKey(dir string) string {
	var r reader
	key, _ := r.claudeJSON(s.ConfigFile).projectKey(dir)
	return key
}

// SetDisabled replaces the disabledMcpServers list of dir's entry in
// ~/.claude.json with what change makes of it, and returns the list before
// and after. It changes no other byte of the file. Claude Code may write the
// file at any moment, so this reads it, makes the change, and replaces the
// file only if it still holds what was read, trying three times. The new file
// keeps the old one's mode, and a link to the file stays a link.
func (s Store) SetDisabled(dir string, change func(current []string) []string) (before, after []string, err error) {
	if !filepath.IsAbs(s.ConfigFile) {
		return nil, nil, errors.New("cannot find ~/.claude.json")
	}
	path := s.ConfigFile
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	for range 3 {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, err
		}
		var top map[string]json.RawMessage
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		cfg := claudeConfig{top: top}
		key, ok := cfg.projectKey(dir)
		if !ok {
			return nil, nil, ErrNoProject
		}
		before = stringsOf(cfg.project(dir)["disabledMcpServers"])
		after = change(slices.Clone(before))
		if after == nil {
			after = []string{}
		}
		if slices.Equal(before, after) {
			return before, after, nil
		}
		out, err := setList(data, []string{"projects", key, "disabledMcpServers"}, after)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		if err := sameButList(data, out, key, after); err != nil {
			return nil, nil, fmt.Errorf("%s: claudes would have changed more than disabledMcpServers, so it wrote nothing: %w", path, err)
		}
		now, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		if !bytes.Equal(now, data) {
			continue
		}
		if err := safefile.WriteFile(path, out, info.Mode().Perm()); err != nil {
			return nil, nil, err
		}
		return before, after, nil
	}
	return nil, nil, errChanged
}

// sameButList checks that out holds what data does, with key's
// disabledMcpServers set to list.
func sameButList(data, out []byte, key string, list []string) error {
	var want, got map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		return err
	}
	if err := json.Unmarshal(out, &got); err != nil {
		return err
	}
	projects, _ := want["projects"].(map[string]any)
	proj, _ := projects[key].(map[string]any)
	if proj == nil {
		return errors.New("no project entry")
	}
	items := make([]any, len(list))
	for i, s := range list {
		items[i] = s
	}
	proj["disabledMcpServers"] = items
	if !reflect.DeepEqual(want, got) {
		return errors.New("the files differ")
	}
	return nil
}

// projectKey is the key Claude Code keeps dir's entry under. Claude Code keys
// a directory in a git repository by the repository's main worktree, and keys
// any other directory by itself.
func (c claudeConfig) projectKey(dir string) (string, bool) {
	projects := objectOf(c.top["projects"])
	if len(projects) == 0 {
		return "", false
	}
	byDir := map[string]string{}
	for key := range projects {
		byDir[configKey(key)] = key
	}
	// Claude Code follows links before it looks for the repository.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	for _, d := range []string{gitRoot(dir), dir} {
		if key, ok := byDir[configKey(d)]; ok && d != "" {
			return key, true
		}
	}
	return "", false
}

// setList sets the list at path, a chain of object keys, in the JSON text
// data, and leaves every other byte as it was. The last key is added when it
// is missing; the others must be there. The list is laid out the way
// Claude Code's JSON.stringify(value, null, 2) lays it out, or on one line in
// a file written on one line.
func setList(data []byte, path []string, list []string) ([]byte, error) {
	sc := scanner{data: data}
	start := sc.space(0)
	// line is where the line that opens the current object starts: its key's,
	// or the object's own at the top.
	line := start
	for i, key := range path {
		obj, err := sc.object(start)
		if err != nil {
			return nil, err
		}
		m, found := obj.member(key)
		if found {
			if i < len(path)-1 {
				start, line = m.value, m.key
				continue
			}
			text := layout(list, sc.indent(m.key))
			return splice(data, m.value, m.end, text), nil
		}
		if i < len(path)-1 {
			return nil, fmt.Errorf("no %q", key)
		}
		name, _ := marshal(key)
		if len(obj.members) == 0 {
			outer := sc.indent(line)
			if outer == nil {
				return splice(data, obj.start+1, obj.end-1, name+":"+layout(list, nil)), nil
			}
			inner := *outer + "  "
			text := "\n" + inner + name + ": " + layout(list, &inner) + "\n" + *outer
			return splice(data, obj.start+1, obj.end-1, text), nil
		}
		last := obj.members[len(obj.members)-1]
		ind := sc.indent(last.key)
		if ind == nil {
			return splice(data, last.end, last.end, ","+name+":"+layout(list, nil)), nil
		}
		return splice(data, last.end, last.end, ",\n"+*ind+name+": "+layout(list, ind)), nil
	}
	return nil, errors.New("no path")
}

func splice(data []byte, from, to int, text string) []byte {
	out := make([]byte, 0, len(data)+len(text))
	out = append(out, data[:from]...)
	out = append(out, text...)
	return append(out, data[to:]...)
}

// layout writes list as JSON. With ind, the indentation of the line its key
// is on, it puts each item on a line of its own; with nil, all on one line.
func layout(list []string, ind *string) string {
	if len(list) == 0 {
		return "[]"
	}
	items := make([]string, len(list))
	for i, s := range list {
		items[i], _ = marshal(s)
	}
	if ind == nil {
		return "[" + strings.Join(items, ",") + "]"
	}
	inner := *ind + "  "
	return "[\n" + inner + strings.Join(items, ",\n"+inner) + "\n" + *ind + "]"
}

// marshal is s as a JSON string, with <, >, and & left as they are, as
// JSON.stringify leaves them.
func marshal(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// scanner finds where values start and end in JSON text that json.Unmarshal
// has already accepted.
type scanner struct{ data []byte }

type member struct {
	// key is where the member's key starts, and value and end bound its
	// value.
	key, value, end int
	name            string
}

type object struct {
	// start is the opening brace and end is just past the closing one.
	start, end int
	members    []member
}

func (o object) member(name string) (member, bool) {
	for _, m := range o.members {
		if m.name == name {
			return m, true
		}
	}
	return member{}, false
}

func (sc scanner) space(i int) int {
	for i < len(sc.data) && strings.IndexByte(" \t\r\n", sc.data[i]) >= 0 {
		i++
	}
	return i
}

// indent is the whitespace before i on its line, or nil when something else
// comes before it there, as in a file written on one line.
func (sc scanner) indent(i int) *string {
	j := i
	for j > 0 && (sc.data[j-1] == ' ' || sc.data[j-1] == '\t') {
		j--
	}
	if j > 0 && sc.data[j-1] != '\n' {
		return nil
	}
	s := string(sc.data[j:i])
	return &s
}

func (sc scanner) object(i int) (object, error) {
	if i >= len(sc.data) || sc.data[i] != '{' {
		return object{}, fmt.Errorf("not an object at byte %d", i)
	}
	obj := object{start: i}
	i = sc.space(i + 1)
	if i < len(sc.data) && sc.data[i] == '}' {
		obj.end = i + 1
		return obj, nil
	}
	for i < len(sc.data) {
		keyEnd, err := sc.str(i)
		if err != nil {
			return object{}, err
		}
		var name string
		if err := json.Unmarshal(sc.data[i:keyEnd], &name); err != nil {
			return object{}, err
		}
		colon := sc.space(keyEnd)
		if colon >= len(sc.data) || sc.data[colon] != ':' {
			return object{}, fmt.Errorf("no colon at byte %d", colon)
		}
		value := sc.space(colon + 1)
		end, err := sc.value(value)
		if err != nil {
			return object{}, err
		}
		obj.members = append(obj.members, member{key: i, value: value, end: end, name: name})
		next := sc.space(end)
		if next >= len(sc.data) {
			break
		}
		switch sc.data[next] {
		case ',':
			i = sc.space(next + 1)
		case '}':
			obj.end = next + 1
			return obj, nil
		default:
			return object{}, fmt.Errorf("unexpected %q at byte %d", sc.data[next], next)
		}
	}
	return object{}, errors.New("unterminated object")
}

// value is the end of the value that starts at i.
func (sc scanner) value(i int) (int, error) {
	if i >= len(sc.data) {
		return 0, errors.New("unexpected end")
	}
	switch sc.data[i] {
	case '"':
		return sc.str(i)
	case '{', '[':
		depth := 0
		for j := i; j < len(sc.data); j++ {
			switch sc.data[j] {
			case '"':
				end, err := sc.str(j)
				if err != nil {
					return 0, err
				}
				j = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return j + 1, nil
				}
			}
		}
		return 0, errors.New("unterminated value")
	}
	j := i
	for j < len(sc.data) && strings.IndexByte(",}] \t\r\n", sc.data[j]) < 0 {
		j++
	}
	return j, nil
}

// str is the end of the string that starts at i, just past its closing quote.
func (sc scanner) str(i int) (int, error) {
	if i >= len(sc.data) || sc.data[i] != '"' {
		return 0, fmt.Errorf("not a string at byte %d", i)
	}
	for j := i + 1; j < len(sc.data); j++ {
		switch sc.data[j] {
		case '\\':
			j++
		case '"':
			return j + 1, nil
		}
	}
	return 0, errors.New("unterminated string")
}
