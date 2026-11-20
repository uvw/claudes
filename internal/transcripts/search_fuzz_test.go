package transcripts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// FuzzSearchPrefilter checks that the needles search looks for before it
// parses a line never skip a line the query matches. It writes text into
// each place search reads, in three JSON encodings, and compares what the
// query finds with the needles and without them.
//
// kind picks where query goes: the words, the pattern, Ran, or Touched, in
// its low two bits. Its other bits pick Tools, Raw, and Who.
func FuzzSearchPrefilter(f *testing.F) {
	for _, s := range []struct {
		text, query string
		kind        uint8
	}{
		{"take the lock", "lock", 0},
		{"a block of code", "lock", 0},
		{"the fileLock field", "lock", 0},
		{"lockfile and blocks", "lock", 0},
		{"see /src/lock.go", "lock", 0},
		{"clock2lock", "lock", 0},
		{"über-lock", "lock", 0},
		{"ünlockable", "lock", 0},
		{"café·lock", "lock", 0},
		{"→lock", "lock", 0},
		{"東京Tokyo", "tokyo", 0},
		{"東京タワーに行く", "タワー", 0},
		{"ソウルの서울에서", "서울", 0},
		{"x\u2028lock", "lock", 0},
		{"a <b> & lock", "<b> & lock", 0},
		{"a\nlock\trace", "lock race", 0},
		{`say "hi" to C:\work, then mention the Café on line` + "\nTWO\tthree\u2028four\u0007five", "line two", 0},
		{`say "hi" to C:\work`, `c:\work`, 0},
		{"ticket ZB-1234 is done", `ZB-\d{4}\b`, 1},
		{"ticket zb-1234 is done", `(?i)zb-\d{4}\b`, 1},
		{"ticket ZB-12 is done", `\d{4}|\bzz`, 1},
		{"a block of code", "lock", 1},
		{"x(ab){2}y", `x(ab){2,}y`, 1},
		{"go test -v ./ls.go", "go test", 2},
		{`C:\x\Repo\main.go`, `c:\x\repo`, 3},
		{"internal/cli/ls.go", "cli/ls", 3},
		{"the lock", "lock", 4},
		{"the lock", "lock", 8},
		{"the lock", "lock", 16},
		{"the lock", "lock", 32},
		{"api_key=sk-ant-0123456789abcdef0123456789abcdef lock", "lock", 0},
		{"\xff\xfelock", "lock", 0},
		{"🙂lock", "lock", 0},
		{"lo\x00ck", "lock", 0},
		{"TODO\x00src", "TODO src", 1},
	} {
		f.Add(s.text, s.query, s.kind)
	}
	f.Fuzz(func(t *testing.T, text, query string, kind uint8) {
		// Go's case folding matches k to the Kelvin sign and s to the long s,
		// which the needles miss, as newNeedle says.
		if strings.ContainsAny(text, "\u212a\u017f") {
			t.Skip()
		}
		q := Query{Now: searchNow, Tools: kind&4 != 0, Raw: kind&8 != 0, Who: []string{"", "you", "claude", ""}[kind>>4&3]}
		switch kind & 3 {
		case 0:
			q.Words = strings.Split(query, "\x00")
		case 1:
			if _, err := regexp.Compile(query); err != nil {
				t.Skip()
			}
			q.Pattern = query
		case 2:
			q.Ran = query
		case 3:
			q.Touched = query
		}
		m, err := compile(q)
		if err != nil {
			t.Skip()
		}
		var data bytes.Buffer
		for _, encode := range []func(any) []byte{goJSON, jsJSON, asciiJSON} {
			for _, r := range searchRecords(text) {
				data.Write(encode(r))
				data.WriteByte('\n')
			}
		}

		var sc scanner
		got := &found{counts: make([][places]int, len(m.terms))}
		sc.chunk(data.Bytes(), m, got)
		every := *m
		every.needles = nil
		want := &found{counts: make([][places]int, len(m.terms))}
		sc.chunk(data.Bytes(), &every, want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("with needles %v\nmatched %v %+v\nwithout, %v %+v\nin\n%s", needleText(m.needles), got.counts, got.matches(), want.counts, want.matches(), data.Bytes())
		}
	})
}

// searchRecords are the records that put text in each place search reads: a
// prompt, a reply, a shell command, an edited path, and a tool's output.
// text's parts, split by NULs, are the blocks of a record that has several,
// or the arguments of a tool call that takes several.
func searchRecords(text string) []any {
	type obj = map[string]any
	parts := strings.Split(text, "\x00")
	user := func(content any) any {
		return obj{"type": "user", "message": obj{"role": "user", "content": content}}
	}
	assistant := func(content ...any) any {
		return obj{"type": "assistant", "message": obj{"role": "assistant", "content": content}}
	}
	var texts []any
	for _, p := range parts {
		texts = append(texts, obj{"type": "text", "text": p})
	}
	return []any{
		user(text),
		user(texts),
		assistant(texts...),
		assistant(obj{"type": "tool_use", "name": "Bash", "input": obj{"command": text}}),
		assistant(obj{"type": "tool_use", "name": "Edit", "input": obj{"file_path": text}}),
		assistant(obj{"type": "tool_use", "name": "Grep", "input": obj{"pattern": parts[0], "path": parts[len(parts)-1]}}),
		user([]any{obj{"type": "tool_result", "content": text}}),
		user([]any{obj{"type": "tool_result", "content": texts}}),
	}
}

// goJSON is how Go writes JSON: <, >, &, U+2028, and U+2029 escaped.
func goJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// jsJSON is how Claude Code writes JSON: only quotes, backslashes, and
// control characters escaped.
func jsJSON(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		panic(err)
	}
	return bytes.ReplaceAll(bytes.ReplaceAll(bytes.TrimSuffix(b.Bytes(), []byte("\n")), []byte(`\u2028`), []byte("\u2028")), []byte(`\u2029`), []byte("\u2029"))
}

// asciiJSON is JSON with every character outside ASCII written as a \u
// escape, a surrogate pair for one outside the Basic Multilingual Plane.
// Every byte outside ASCII is inside a string, so each can be escaped.
func asciiJSON(v any) []byte {
	var b bytes.Buffer
	for _, r := range string(goJSON(v)) {
		if r < utf8.RuneSelf {
			b.WriteRune(r)
			continue
		}
		for _, u := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&b, `\u%04x`, u)
		}
	}
	return b.Bytes()
}

func needleText(needles []*needle) []string {
	var out []string
	for _, n := range needles {
		out = append(out, fmt.Sprintf("%q start=%v", n.lower, n.wordStart))
	}
	return out
}
