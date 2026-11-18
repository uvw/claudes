package transcripts

import (
	"bytes"
	"os"
)

// Follower reads a transcript as Claude Code appends to it. A line still being
// written is held back until it ends.
type Follower struct {
	path   string
	offset int64
	// head is the transcript's first line, which a rewrite changes.
	head []byte
}

// Follow opens the transcript at path and returns the events it holds so far.
func Follow(path string) (*Follower, []Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	done := bytes.LastIndexByte(data, '\n') + 1
	f := &Follower{path: path, offset: int64(done), head: headOf(data[:done])}
	return f, Events(data[:done]), nil
}

// Next returns the events written since the last read. rewritten is true when
// Claude Code rewrote the transcript, such as on a compaction; the events then
// start from its beginning.
func (f *Follower) Next() (events []Event, rewritten bool, err error) {
	all, err := os.ReadFile(f.path)
	if err != nil {
		return nil, false, err
	}
	if int64(len(all)) < f.offset || !bytes.Equal(headOf(all), f.head) {
		rewritten, f.offset = true, 0
	}
	more := all[f.offset:]
	end := bytes.LastIndexByte(more, '\n') + 1
	f.offset += int64(end)
	if f.head == nil {
		f.head = headOf(all[:f.offset])
	}
	return Events(more[:end]), rewritten, nil
}

// headOf is the first line of a transcript. A transcript without a whole line
// has none yet.
func headOf(data []byte) []byte {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return bytes.Clone(data[:i])
	}
	return nil
}
