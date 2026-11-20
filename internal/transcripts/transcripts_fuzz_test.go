package transcripts

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FuzzTranscript reads any bytes as a transcript, as session ls, logs,
// search, and gc do, and checks that none of them panics. A transcript is
// Empty only when every line is a bookkeeping record without a message, and
// its tally is Complete only when every line parses and has a known type.
func FuzzTranscript(f *testing.F) {
	for _, lines := range [][]string{
		{`{"type":"mode","sessionId":"` + sid + `","cwd":"/a"}`, `{"type":"file-history-snapshot"}`, ``},
		{`{"type":"user","message":{"role":"user","content":"fix the build"},"cwd":"/a","timestamp":"2026-11-14T10:00:00Z"}`},
		{`{"type":"mode"}`, `not json`},
		{`{"type":"mode","message":null}`},
		{`{"type":"mode","TYPE":null}`, `{"type":"mode","Type":"user","type":"custom-title"}`},
		{`{"type":"user","meſſage":{}}`},
		{`{"type":1}`, `null`, `[]`, `"x"`, "\r", "{\"type\":\"mode\"}\r"},
		{`{"type":"new-kind"}`},
		{`{"type":"user","isSidechain":true,"message":{"content":"look"}}`, `{"type":"weird","isMeta":true}`},
		{`{"type":"ai-title","aiTitle":"Fix tests"}`, `{"type":"custom-title","customTitle":"mine"}`, `{"type":"relocated","relocatedCwd":"/c"}`},
		{`{"type":"assistant","message":{"content":[{"type":"text","text":"On it."},{"type":"tool_use","name":"Bash","input":{"command":"go test ./...\nmore"}},{"type":"tool_use","name":"Edit","input":{"file_path":"x.go"}}]}}`},
		{`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"text","text":"ok"}]}]}}`},
		{`{"type":"user","message":{"content":"<teammate-message teammate_id=\"tester\">{\"type\":\"idle_notification\",\"summary\":\"done\"}</teammate-message>"}}`},
		{`{"type":"user","message":{"content":"<teammate-message teammate_id=\"a\">unclosed"}}`},
		{`{"type":"user","message":{"content":"<command-name>/clear</command-name>"}}`, `{"type":"system","subtype":"turn_duration","durationMs":3044}`},
		{`{"type":"user","message":{"content":[{"type":"image","source":{}}]}}`, `{"type":"user","message":{"content":"[Request interrupted by user]"}}`},
		{`{"type":"user","message":{"content":"api_key=sk-ant-0123456789abcdef0123456789abcdef lock"}}`},
		{"{\"type\":\"user\",\"message\":{\"content\":\"\xff\xfe lock\"}}"},
	} {
		f.Add([]byte(strings.Join(lines, "\n")))
	}
	path := filepath.Join(f.TempDir(), "-a", sid+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.Fatal(err)
	}
	var matchers []*matcher
	for _, q := range []Query{
		{Words: []string{"lock"}, Tools: true},
		{Words: []string{"the build", "x"}, Who: "you"},
		{Pattern: `(?i)o[nk]`, Ran: "go", Touched: "x.go"},
	} {
		m, err := compile(q)
		if err != nil {
			f.Fatal(err)
		}
		matchers = append(matchers, m)
	}
	var sc scanner
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		s, err := summarize(path)
		if err != nil {
			t.Fatal(err)
		}
		if s.Empty {
			for line := range bytes.SplitSeq(data, []byte{'\n'}) {
				if r, ok := decodeTop(line); len(bytes.TrimSpace(line)) > 0 && (!ok || r.message || !bookkeeping[r.typ]) {
					t.Fatalf("Empty, but %q is not a bookkeeping record without a message", line)
				}
			}
		}

		tally, err := ReadTally(path)
		if err != nil {
			t.Fatal(err)
		}
		if tally.Complete() {
			for line := range bytes.Lines(data) {
				r, ok := decodeTop(line)
				// A meta, compaction, or subagent record is skipped whatever
				// its type, since none of them is the user's work.
				if len(bytes.TrimSpace(line)) > 0 && (!ok || !r.skipped && !known[r.typ]) {
					t.Fatalf("Complete, but %q did not parse or has a type claudes does not know", line)
				}
			}
		}

		Events(data)
		for _, m := range matchers {
			found := &found{counts: make([][places]int, len(m.terms))}
			if err := sc.file(path, m, found); err != nil {
				t.Fatal(err)
			}
			m.score(found, searchNow, searchNow)
			found.matches()
		}
	})
}

// topRecord is what decodeTop reads from a transcript line.
type topRecord struct {
	typ     string
	message bool
	// skipped is a meta, compaction, or subagent record.
	skipped bool
}

// decodeTop reads a line as encoding/json fills a struct: a key matches a
// field in any case, a later key wins, and null leaves a field alone. ok is
// false for anything but one JSON object, or a type or flag of the wrong
// kind.
func decodeTop(line []byte) (r topRecord, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return r, false
	}
	flags := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return r, false
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return r, false
		}
		null := string(value) == "null"
		switch {
		case strings.EqualFold(key, "message"):
			r.message = true
		case strings.EqualFold(key, "type") && !null:
			if json.Unmarshal(value, &r.typ) != nil {
				return r, false
			}
		case strings.EqualFold(key, "isMeta") || strings.EqualFold(key, "isCompactSummary") || strings.EqualFold(key, "isSidechain"):
			var flag bool
			if !null && json.Unmarshal(value, &flag) != nil {
				return r, false
			}
			if !null {
				flags[strings.ToLower(key)] = flag
			}
		}
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return r, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return r, false
	}
	r.skipped = flags["ismeta"] || flags["iscompactsummary"] || flags["issidechain"]
	return r, true
}
