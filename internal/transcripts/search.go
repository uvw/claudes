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
	// Words match in any case at the start of a word, so lock matches locks
	// and fileLock but not block. White space in a word matches any run of
	// it, such as a line break.
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
	// Raw keeps secrets in the snippets. Without it, Redact masks them.
	// Either way, the terms match the text as written.
	Raw bool
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
	// Matches are the best few records in each place, the best first.
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
	// prompts, and so on.
	placeWeights = [places]float64{16, 8, 4, 2, 1}
)

// A session scores its best record's place weight times how close the query
// came in that record, plus spreadShare of what every term matched across
// the session. Closeness counts for more than the spread, so one prompt that
// holds every word outranks a session whose words are scattered over many.
const (
	// closePhrase is a record that holds the words next to each other, in
	// order, and closeAll one that holds every term. A record that holds some
	// scores the share it holds, below 1.
	closePhrase = 3
	closeAll    = 2
	spreadShare = 0.25
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
	// wordStart is whether a match must start a word.
	wordStart bool
}

// find is where t first matches text, or nil.
func (t term) find(text string) []int {
	for off := 0; off <= len(text); {
		loc := t.re.FindStringIndex(text[off:])
		if loc == nil {
			return nil
		}
		start, end := off+loc[0], off+loc[1]
		if !t.wordStart || startsWord(text, start) {
			return []int{start, end}
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		off = start + max(size, 1)
	}
	return nil
}

// startsWord reports whether a word starts at text[i]: at the start of text,
// after a character that is not a letter or a digit, such as a space, a
// slash, or an underscore, at an upper-case letter after a lower-case one, as
// in camelCase, or next to a script written without spaces between words.
func startsWord(text string, i int) bool {
	if i == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(text[:i])
	next, _ := utf8.DecodeRuneInString(text[i:])
	switch {
	case !unicode.IsLetter(prev) && !unicode.IsDigit(prev), unspaced(prev), unspaced(next):
		return true
	}
	return unicode.IsLower(prev) && unicode.IsUpper(next)
}

// unspaced reports whether r belongs to a script written without spaces
// between words, where any character can start one. The needle a word-start
// term uses is ASCII, and such a character before it is a byte of a UTF-8
// sequence or a \u escape, which mayStartWord already lets through.
func unspaced(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai,
		unicode.Lao, unicode.Khmer, unicode.Myanmar)
}

// matcher is a compiled query.
type matcher struct {
	terms []term
	// phrase matches all the words next to each other, in order, or is nil
	// when there is only one.
	phrase *term
	// places are where the words and the pattern match.
	places [places]bool
	// needles are what a line must hold one of to be parsed, or nil when
	// every line must be.
	needles []*needle
	raw     bool
}

func compile(q Query) (*matcher, error) {
	m := &matcher{raw: q.Raw}
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
	// A literal term compiles to a regular expression, which must be UTF-8.
	for _, s := range append([]string{q.Ran, q.Touched}, q.Words...) {
		if !utf8.ValidString(s) {
			return nil, fmt.Errorf("%q is not valid UTF-8", s)
		}
	}
	var words []string
	for _, w := range q.Words {
		if t, ok := literal(strings.Fields(w), anywhere, "", true); ok {
			m.terms = append(m.terms, t)
			words = append(words, strings.Fields(w)...)
		}
	}
	if len(words) > 1 {
		m.phrase = new(term)
		*m.phrase, _ = literal(words, anywhere, "", true)
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
	if t, ok := literal(strings.Fields(q.Ran), ranCommand, "", false); ok {
		m.terms = append(m.terms, t)
	}
	// A Windows path's separators are escaped in the transcript, so the
	// needle stops at them.
	if t, ok := literal(strings.Fields(slashed(q.Touched)), touchedPath, "/", false); ok {
		m.terms = append(m.terms, t)
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

// literal matches parts in any case, in order, with any run of white space
// between them. With wordStart, a match must start a word, unless the first
// part starts with something else, such as the dot of .go.
func literal(parts []string, sc scope, cut string, wordStart bool) (term, bool) {
	if len(parts) == 0 {
		return term{}, false
	}
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = regexp.QuoteMeta(p)
	}
	first, _ := utf8.DecodeRuneInString(parts[0])
	t := term{
		re:        regexp.MustCompile(`(?i)` + strings.Join(quoted, `\s+`)),
		needle:    newNeedle(parts, cut),
		scope:     sc,
		wordStart: wordStart && (unicode.IsLetter(first) || unicode.IsDigit(first)),
	}
	// Every part starts a word: the first one when the term must, and the
	// rest after white space. So does a needle that starts a part.
	if t.needle != nil {
		t.needle.wordStart = t.wordStart && t.needle.prefix
	}
	return t, true
}

// needle is text to find in a transcript line, ignoring ASCII case.
type needle struct {
	lower []byte
	// at is the byte of lower that is searched for first: the one least
	// likely to appear in a transcript, so there are few false starts.
	at int
	// prefix is whether lower starts the literal it came from, and wordStart
	// whether it must start a word.
	prefix, wordStart bool
}

// newNeedle is the longest run of the strings that is spelled the same in a
// transcript line as in the text it decodes to: printable ASCII that JSON
// does not escape, without the characters in cut. It is nil when there is no
// such run. Go's case folding also matches k to the Kelvin sign and s to the
// long s, which the needle misses.
func newNeedle(literals []string, cut string) *needle {
	split := func(r rune) bool { return r >= utf8.RuneSelf || !plain(byte(r)) || strings.ContainsRune(cut, r) }
	best, prefix := "", false
	for _, s := range literals {
		for _, run := range strings.FieldsFunc(s, split) {
			if len(run) > len(best) {
				best, prefix = run, strings.HasPrefix(s, run)
			}
		}
	}
	if best == "" {
		return nil
	}
	n := makeNeedle(best)
	n.prefix = prefix
	return n
}

// mayStartWord reports whether the text that data[at] begins in a transcript
// line can start a word once decoded. It may say yes when the word does not
// start there, never the reverse.
func mayStartWord(data []byte, at int) bool {
	if at == 0 {
		return true
	}
	prev := data[at-1]
	alnum := func(c byte) bool { return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' }
	switch {
	case !alnum(prev), 'a' <= prev && prev <= 'z' && 'A' <= data[at] && data[at] <= 'Z':
		return true
	}
	// An escape, such as \n or a \u with four hex digits, ends in a letter
	// or a digit, and decodes to a character that need not be one.
	return at >= 2 && data[at-2] == '\\' || at >= 6 && data[at-6] == '\\' && data[at-5] == 'u'
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
	// counts are each term's matching records in each place.
	counts [][places]int
	// best are the closest records in each place.
	best [places][]hit
	read int
}

// hit is a record that matched.
type hit struct {
	place     int
	closeness float64
	// seq orders records as read, so the earlier of two as close is kept.
	seq   int
	match Match
}

func (h hit) score() float64 { return placeWeights[h.place] * h.closeness }

// field is one text of a record, which the terms of its scope match.
type field struct {
	sc   scope
	text string
}

// look matches one record, found in place, whose texts are fields. label,
// such as a tool's name, starts its snippet.
func (m *matcher) look(f *found, place int, label string, fields ...field) {
	searched := func(fl field) bool { return fl.text != "" && (fl.sc != anywhere || m.places[place]) }
	matched := 0
	var at []int
	atField := 0
	for i, t := range m.terms {
		for j, fl := range fields {
			if fl.sc != t.scope || !searched(fl) {
				continue
			}
			if loc := t.find(fl.text); loc != nil {
				f.counts[i][place]++
				matched++
				// The snippet shows the first match in the first field.
				if at == nil || j < atField || j == atField && loc[0] < at[0] {
					at, atField = loc, j
				}
				break
			}
		}
	}
	if matched == 0 {
		return
	}
	closeness := float64(matched) / float64(len(m.terms))
	if matched == len(m.terms) {
		closeness = closeAll
		for j, fl := range fields {
			if m.phrase == nil || fl.sc != anywhere || !searched(fl) {
				continue
			}
			if loc := m.phrase.find(fl.text); loc != nil {
				closeness, at, atField = closePhrase, loc, j
				break
			}
		}
	}
	f.read++
	h := hit{place: place, closeness: closeness, seq: f.read}
	kept := f.best[place]
	if len(kept) == keepSnippets {
		worst := 0
		for i, k := range kept {
			if k.closeness < kept[worst].closeness || k.closeness == kept[worst].closeness && k.seq > kept[worst].seq {
				worst = i
			}
		}
		if closeness <= kept[worst].closeness {
			return
		}
		kept = slices.Delete(kept, worst, worst+1)
	}
	text, start, end := fields[atField].text, at[0], at[1]
	if !m.raw {
		text, start, end = redactAround(text, start, end)
	}
	s := snippet(text, start, end)
	if label != "" {
		s = label + ": " + s
	}
	h.match = Match{Where: placeNames[place], Snippet: s}
	f.best[place] = append(kept, h)
}

// matches are the records kept, the best first.
func (f *found) matches() []Match {
	var hits []hit
	for _, kept := range f.best {
		hits = append(hits, kept...)
	}
	slices.SortStableFunc(hits, func(a, b hit) int {
		return cmp.Or(cmp.Compare(b.score(), a.score()), cmp.Compare(a.place, b.place), cmp.Compare(a.seq, b.seq))
	})
	out := make([]Match, len(hits))
	for i, h := range hits {
		out[i] = h.match
	}
	return out
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
			m.look(f, inYou, "", field{anywhere, typed})
		case userToolResult:
			for _, b := range bs {
				if b.Type == "tool_result" {
					m.look(f, inOutput, "", field{anywhere, resultText(b.Content)})
				}
			}
		}
	case "assistant":
		for _, b := range blocks(r.Message) {
			switch b.Type {
			case "text":
				m.look(f, inClaude, "", field{anywhere, b.Text})
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
	// One argument per line, so no needle, which holds no line break, can
	// match across two of them.
	call := []field{{anywhere, strings.Join(args, "\n")}}
	if b.Name == "Bash" {
		call = append(call, field{ranCommand, in.Command})
	}
	if editTools[b.Name] {
		call = append(call, field{touchedPath, slashed(cmp.Or(in.FilePath, in.NotebookPath))})
	}
	m.look(f, inTool, b.Name, call...)
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
// The spread grows with half the log of each count, so it takes a hundred
// records in one place to equal one in the place above.
func (m *matcher) score(f *found, active, now time.Time) (float64, bool) {
	spread := 0.0
	for _, counts := range f.counts {
		matched := false
		for place, n := range counts {
			if n > 0 {
				matched = true
				spread += placeWeights[place] * (1 + math.Log10(float64(n))/2)
			}
		}
		if !matched {
			return 0, false
		}
	}
	best := 0.0
	for _, kept := range f.best {
		for _, h := range kept {
			best = max(best, h.score())
		}
	}
	total := best + spreadShare*spread/float64(len(f.counts))
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
			if nd.wordStart && !mayStartWord(data, at) {
				off = at + 1
				continue
			}
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
				m.look(f, inName, "", field{anywhere, s.Name})
				if sc.file(s.Path, m, f) != nil {
					continue
				}
				if score, ok := m.score(f, s.LastActive, q.Now); ok {
					results[i] = &Result{Session: s, Score: score, Matches: f.matches()}
				}
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
