package transcripts

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"regexp/syntax"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Query is what session search looks for. A session matches when each word,
// the pattern, Ran, and Touched all match somewhere in it.
type Query struct {
	// Words match as case-insensitive substrings. White space in a word
	// matches any run of it, such as a line break.
	Words []string
	// Pattern is a regular expression in Go's RE2 syntax, or "".
	Pattern string
	// Who limits the words and the pattern to the user's prompts (you) or
	// Claude's replies (claude). With "", they match the name, the prompts,
	// the replies, and the tool calls.
	Who string
	// Tools also searches what the tools returned.
	Tools bool
	// Ran matches only the shell commands Claude ran.
	Ran string
	// Touched matches only the paths of the files Claude edited or wrote.
	Touched string
	// Now is when recency is measured from.
	Now time.Time
}

// Match is one place a query matched.
type Match struct {
	// Where is name, you, claude, tool, or output.
	Where   string `json:"where"`
	Snippet string `json:"snippet"`
}

// Result is a session that matched, with its score.
type Result struct {
	Session
	Score float64
	// Matches are the first few in each place, the best place first.
	Matches []Match
}

// The places a query looks in, from the one that counts most.
const (
	inName = iota
	inYou
	inClaude
	inTool
	inOutput
	places
)

var (
	placeNames = [places]string{"name", "you", "claude", "tool", "output"}
	// placeWeights rank a match in a session's name above one in the user's
	// prompts, and so on. Within a place, the score grows with half the log
	// of the count, so it takes a hundred matches in one place to equal a
	// single match in the place above.
	placeWeights = [places]float64{16, 8, 4, 2, 1}
)

const (
	// recencyBoost is how much more a session active just now scores, at most;
	// it halves every recencyHalfLife.
	recencyBoost    = 0.25
	recencyHalfLife = 14 * 24 * time.Hour
	// keepSnippets is how many matches Search keeps from each place.
	keepSnippets = 3
	chunkSize    = 1 << 20
)

// scope is which text a term matches.
type scope uint8

const (
	anywhere scope = iota
	ranCommand
	touchedPath
)

type term struct {
	re *regexp.Regexp
	// needle is what a transcript line must hold before the term can match
	// it, or nil when any line can.
	needle *needle
	scope  scope
}

// matcher is a compiled query.
type matcher struct {
	terms []term
	// places are where the words and the pattern match.
	places [places]bool
	// needles are what a line must hold one of to be parsed, or nil when
	// every line must be.
	needles []*needle
}

func compile(q Query) (*matcher, error) {
	m := &matcher{}
	switch q.Who {
	case "":
		m.places = [places]bool{inName: true, inYou: true, inClaude: true, inTool: true, inOutput: q.Tools}
	case "you":
		m.places[inYou] = true
	case "claude":
		m.places[inClaude] = true
	default:
		return nil, fmt.Errorf("who is you or claude, not %q", q.Who)
	}
	literal := func(s string, sc scope, cut string) {
		parts := strings.Fields(s)
		if len(parts) == 0 {
			return
		}
		quoted := make([]string, len(parts))
		for i, p := range parts {
			quoted[i] = regexp.QuoteMeta(p)
		}
		re := regexp.MustCompile(`(?i)` + strings.Join(quoted, `\s+`))
		m.terms = append(m.terms, term{re: re, needle: newNeedle(parts, cut), scope: sc})
	}
	for _, w := range q.Words {
		literal(w, anywhere, "")
	}
	if q.Pattern != "" {
		re, err := regexp.Compile(q.Pattern)
		if err != nil {
			return nil, err
		}
		parsed, err := syntax.Parse(q.Pattern, syntax.Perl)
		if err != nil {
			return nil, err
		}
		m.terms = append(m.terms, term{re: re, needle: newNeedle(required(parsed), ""), scope: anywhere})
	}
	if q.Ran != "" {
		literal(q.Ran, ranCommand, "")
	}
	if q.Touched != "" {
		// A Windows path's separators are escaped in the transcript, so the
		// needle stops at them.
		literal(slashed(q.Touched), touchedPath, "/")
	}
	if len(m.terms) == 0 {
		return nil, errors.New("nothing to search for")
	}
	for _, t := range m.terms {
		if t.needle == nil {
			m.needles = nil
			break
		}
		m.needles = append(m.needles, t.needle)
	}
	return m, nil
}

// needle is text to find in a transcript line, ignoring ASCII case.
type needle struct {
	lower []byte
	// at is the byte of lower that is searched for first: the one least
	// likely to appear in a transcript, so there are few false starts.
	at int
}

// newNeedle is the longest run of the strings that is spelled the same in a
// transcript line as in the text it decodes to: printable ASCII that JSON
// does not escape, without the characters in cut. It is nil when there is no
// such run. Go's case folding also matches k to the Kelvin sign and s to the
// long s, which the needle misses.
func newNeedle(literals []string, cut string) *needle {
	split := func(r rune) bool { return r >= utf8.RuneSelf || !plain(byte(r)) || strings.ContainsRune(cut, r) }
	best := ""
	for _, s := range literals {
		for _, run := range strings.FieldsFunc(s, split) {
			if len(run) > len(best) {
				best = run
			}
		}
	}
	if best == "" {
		return nil
	}
	return makeNeedle(best)
}

func makeNeedle(s string) *needle {
	n := &needle{lower: []byte(strings.ToLower(s))}
	for i, c := range n.lower {
		if rarity(c) > rarity(n.lower[n.at]) {
			n.at = i
		}
	}
	return n
}

// commonFirst are the bytes most frequent in transcripts, most frequent first.
const commonFirst = `":, etaoinsrlcdhu1023456789mpf.gy_bw-/v{}k[]x()jzq`

// rarity is how seldom c appears in a transcript, roughly.
func rarity(c byte) int {
	if i := strings.IndexByte(commonFirst, c); i >= 0 {
		return i
	}
	return len(commonFirst)
}

// index is the offset of the needle's first match in data, or -1.
func (n *needle) index(data []byte) int {
	lo := n.lower[n.at]
	up := lo
	if 'a' <= lo && lo <= 'z' {
		up = lo - 'a' + 'A'
	}
	nextLo, nextUp := -1, -1
	for from := n.at; ; {
		// IndexByte finds each case of the rare byte much faster than
		// comparing every byte would.
		if nextLo < from {
			nextLo = indexFrom(data, lo, from)
		}
		if nextUp < from {
			nextUp = indexFrom(data, up, from)
		}
		c := min(nextLo, nextUp)
		if c == len(data) {
			return -1
		}
		start := c - n.at
		if end := start + len(n.lower); end <= len(data) && equalFoldASCII(data[start:end], n.lower) {
			return start
		}
		from = c + 1
	}
}

// indexFrom is where c next appears in data at or after from, or len(data).
func indexFrom(data []byte, c byte, from int) int {
	if from >= len(data) {
		return len(data)
	}
	if i := bytes.IndexByte(data[from:], c); i >= 0 {
		return from + i
	}
	return len(data)
}

// equalFoldASCII reports whether b equals lower, which is in lower case,
// ignoring ASCII case.
func equalFoldASCII(b, lower []byte) bool {
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

// plain reports whether c is written as is in JSON. Claude Code escapes only
// quotes, backslashes, and control characters; Go's encoder also escapes <,
// >, and &.
func plain(c byte) bool {
	return c >= 0x20 && c < 0x7f && !strings.ContainsRune(`"\<>&`, rune(c))
}

// required are literal strings every match of re contains.
func required(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		return []string{string(re.Rune)}
	case syntax.OpCapture, syntax.OpPlus:
		return required(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min > 0 {
			return required(re.Sub[0])
		}
	case syntax.OpConcat:
		var out []string
		for _, sub := range re.Sub {
			out = append(out, required(sub)...)
		}
		return out
	}
	return nil
}

func slashed(path string) string { return strings.ReplaceAll(path, `\`, "/") }

// found is what one session's transcript matched.
type found struct {
	// counts are each term's matches in each place.
	counts   [][places]int
	snippets [places][]Match
}

// look matches text, found in place, against the terms of scope sc. label,
// such as a tool's name, starts the snippet.
func (m *matcher) look(f *found, place int, sc scope, label, text string) {
	if text == "" || sc == anywhere && !m.places[place] {
		return
	}
	saved := false
	for i, t := range m.terms {
		if t.scope != sc {
			continue
		}
		loc := t.re.FindStringIndex(text)
		if loc == nil {
			continue
		}
		f.counts[i][place]++
		if !saved && len(f.snippets[place]) < keepSnippets {
			s := snippet(text, loc[0], loc[1])
			if label != "" {
				s = label + ": " + s
			}
			f.snippets[place] = append(f.snippets[place], Match{Where: placeNames[place], Snippet: s})
			saved = true
		}
	}
}

// The needles that tell a record's type without parsing it. A JSON string
// escapes its quotes, so in a line written without spaces, as Claude Code
// writes them, these appear only as keys and values.
var (
	anyType        = makeNeedle(`"type":"`)
	userType       = makeNeedle(`"type":"user"`)
	assistantType  = makeNeedle(`"type":"assistant"`)
	toolResultType = makeNeedle(`"type":"tool_result"`)
)

// record matches one transcript line. Only the user's prompts, Claude's text
// and tool calls, and, with Tools, the tools' output count. Text Claude Code
// adds, such as reminders and compaction summaries, does not.
func (m *matcher) record(line []byte, f *found) {
	// Most of a transcript is tool results and records of other types, such
	// as attachments and API requests, which are skipped without parsing.
	if anyType.index(line) >= 0 && userType.index(line) < 0 && assistantType.index(line) < 0 ||
		!m.places[inOutput] && toolResultType.index(line) >= 0 {
		return
	}
	var r activityRecord
	if json.Unmarshal(line, &r) != nil || r.IsMeta || r.IsCompactSummary || r.IsSidechain {
		return
	}
	switch r.Type {
	case "user":
		bs := blocks(r.Message)
		switch kind, typed, _ := classifyUser(bs); kind {
		case userPrompt:
			m.look(f, inYou, anywhere, "", typed)
		case userToolResult:
			for _, b := range bs {
				if b.Type == "tool_result" {
					m.look(f, inOutput, anywhere, "", resultText(b.Content))
				}
			}
		}
	case "assistant":
		for _, b := range blocks(r.Message) {
			switch b.Type {
			case "text":
				m.look(f, inClaude, anywhere, "", b.Text)
			case "tool_use":
				m.toolCall(f, b)
			}
		}
	}
}

// toolInput is the part of a tool call's input search reads: commands, paths,
// and patterns, not the text a tool writes.
type toolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	Pattern      string `json:"pattern"`
	Glob         string `json:"glob"`
	URL          string `json:"url"`
	Query        string `json:"query"`
}

func (m *matcher) toolCall(f *found, b block) {
	var in toolInput
	// A field of another type is left empty, and the rest are still read.
	_ = json.Unmarshal(b.Input, &in)
	var args []string
	for _, v := range []string{in.Command, in.FilePath, in.NotebookPath, in.Path, in.Pattern, in.Glob, in.URL, in.Query} {
		if v != "" {
			args = append(args, v)
		}
	}
	m.look(f, inTool, anywhere, b.Name, strings.Join(args, " "))
	if b.Name == "Bash" {
		m.look(f, inTool, ranCommand, b.Name, in.Command)
	}
	if editTools[b.Name] {
		m.look(f, inTool, touchedPath, b.Name, slashed(cmp.Or(in.FilePath, in.NotebookPath)))
	}
}

// resultText is a tool result's text: a string, or the text of its blocks.
func resultText(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var bs []block
	_ = json.Unmarshal(content, &bs)
	var out strings.Builder
	for _, b := range bs {
		if b.Type == "text" {
			out.WriteString(b.Text)
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// score is a session's score, or false when a term matched nothing in it.
func (m *matcher) score(f *found, active, now time.Time) (float64, bool) {
	total := 0.0
	for _, counts := range f.counts {
		matched := false
		for place, n := range counts {
			if n > 0 {
				matched = true
				total += placeWeights[place] * (1 + math.Log10(float64(n))/2)
			}
		}
		if !matched {
			return 0, false
		}
	}
	age := max(now.Sub(active), 0)
	total *= 1 + recencyBoost*math.Exp2(-float64(age)/float64(recencyHalfLife))
	return math.Round(total*100) / 100, true
}

// scanner reads transcripts for one worker, reusing its buffers.
type scanner struct {
	buf   []byte
	lines [][2]int
}

// file matches every line of the transcript at path.
func (sc *scanner) file(path string, m *matcher, f *found) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	if sc.buf == nil {
		sc.buf = make([]byte, chunkSize)
	}
	kept := 0
	for {
		n, err := io.ReadFull(fh, sc.buf[kept:])
		data := sc.buf[:kept+n]
		done := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
		if err != nil && !done {
			return err
		}
		end := len(data)
		if !done {
			if end = bytes.LastIndexByte(data, '\n') + 1; end == 0 {
				// One line fills the buffer.
				sc.buf = append(sc.buf, make([]byte, len(sc.buf))...)
				kept = len(data)
				continue
			}
		}
		sc.chunk(data[:end], m, f)
		if done {
			return nil
		}
		kept = copy(sc.buf, data[end:])
	}
}

// chunk matches the whole lines in data. Only a line that holds a needle is
// parsed.
func (sc *scanner) chunk(data []byte, m *matcher, f *found) {
	if m.needles == nil {
		for line := range bytes.SplitSeq(data, []byte{'\n'}) {
			m.record(line, f)
		}
		return
	}
	sc.lines = sc.lines[:0]
	for _, nd := range m.needles {
		for off := 0; off < len(data); {
			i := nd.index(data[off:])
			if i < 0 {
				break
			}
			at := off + i
			start, end := bytes.LastIndexByte(data[:at], '\n')+1, len(data)
			if j := bytes.IndexByte(data[at:], '\n'); j >= 0 {
				end = at + j
			}
			sc.lines = append(sc.lines, [2]int{start, end})
			off = end + 1
		}
	}
	slices.SortFunc(sc.lines, func(a, b [2]int) int { return cmp.Compare(a[0], b[0]) })
	for _, l := range slices.Compact(sc.lines) {
		m.record(data[l[0]:l[1]], f)
	}
}

// Search reads the transcripts of list in full, in parallel, and returns the
// sessions that match q, best first. A transcript that cannot be read is left
// out.
func Search(list []Session, q Query) ([]Result, error) {
	m, err := compile(q)
	if err != nil {
		return nil, err
	}
	// The biggest first, so no worker is left with one at the end.
	order := make([]int, len(list))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int { return cmp.Compare(list[b].Size, list[a].Size) })

	results := make([]*Result, len(list))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Go(func() {
			var sc scanner
			for i := range next {
				s := list[i]
				f := &found{counts: make([][places]int, len(m.terms))}
				m.look(f, inName, anywhere, "", s.Name)
				if sc.file(s.Path, m, f) != nil {
					continue
				}
				score, ok := m.score(f, s.LastActive, q.Now)
				if !ok {
					continue
				}
				r := &Result{Session: s, Score: score, Matches: []Match{}}
				for _, ms := range f.snippets {
					r.Matches = append(r.Matches, ms...)
				}
				results[i] = r
			}
		})
	}
	for _, i := range order {
		next <- i
	}
	close(next)
	wg.Wait()

	var out []Result
	for _, r := range results {
		if r != nil {
			out = append(out, *r)
		}
	}
	slices.SortStableFunc(out, func(a, b Result) int {
		return cmp.Or(cmp.Compare(b.Score, a.Score), b.LastActive.Compare(a.LastActive))
	})
	return out, nil
}

const (
	// snippetBefore is how many characters of context a snippet keeps before
	// the match, and snippetLength how long it is at most.
	snippetBefore = 24
	snippetLength = 72
)

// snippet is the match at text[start:end] in context, on one line.
func snippet(text string, start, end int) string {
	lo := max(start-4*snippetBefore, 0)
	for lo < start && !utf8.RuneStart(text[lo]) {
		lo++
	}
	hi := min(end+4*snippetLength, len(text))
	for hi > end && hi < len(text) && !utf8.RuneStart(text[hi]) {
		hi--
	}
	before, match, after := []rune(squash(text[lo:start])), []rune(squash(text[start:end])), []rune(squash(text[end:hi]))
	cutStart, cutEnd := lo > 0, hi < len(text)
	if len(before) > snippetBefore {
		before, cutStart = before[len(before)-snippetBefore:], true
	}
	if len(match) > snippetLength {
		match, after, cutEnd = match[:snippetLength], nil, true
	}
	if room := max(snippetLength-len(before)-len(match), 0); len(after) > room {
		after, cutEnd = after[:room], true
	}
	// A cut is made between words, when the context has a space to cut at.
	if i := slices.Index(before, ' '); cutStart && i >= 0 {
		before = before[i+1:]
	}
	if cutEnd {
		for i := len(after) - 1; i >= 0; i-- {
			if after[i] == ' ' {
				after = after[:i]
				break
			}
		}
	}
	s := strings.TrimSpace(string(before) + string(match) + string(after))
	if cutStart {
		s = "…" + s
	}
	if cutEnd {
		s += "…"
	}
	return s
}

// squash makes each run of white space in s a single space.
func squash(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}
