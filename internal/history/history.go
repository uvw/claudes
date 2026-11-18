// Package history judges past sessions as claudes session shows them: what
// each transcript says, which sessions session gc removes and why, how triage
// sorts them, the space they take, and what a search found.
package history

import (
	"cmp"

	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
)

// Session is a session as claudes session shows it: what its transcript
// says, and whether it runs now.
type Session struct {
	ShortID string `json:"id"`
	transcripts.Session
	Running bool `json:"running"`
	// Disk is the space the session takes on disk.
	Disk transcripts.Disk `json:"disk"`
	// Activity is what the session did, or nil when its transcript was not
	// read in full.
	Activity *transcripts.Tally `json:"activity"`
	// tallyErr is why reading the activity failed.
	tallyErr error
}

// ReadTally counts what p did, unless its transcript is bigger than
// transcripts.TallyLimit, or force reads it whatever its size.
func (p *Session) ReadTally(force bool) {
	if p.Activity != nil || p.tallyErr != nil || !force && p.Size > transcripts.TallyLimit {
		return
	}
	a, err := transcripts.ReadTally(p.Path)
	if err != nil {
		p.tallyErr = err
		return
	}
	p.Activity = &a
}

// Ref is how the JSON of session du and session triage names a session.
type Ref struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

// Label is the session's name, or its short id when it has none.
func (r Ref) Label() string { return cmp.Or(r.Name, r.ID) }

// Ref is how JSON names p.
func (p Session) Ref() Ref {
	return Ref{ID: p.ShortID, SessionID: p.SessionID, Name: p.Name}
}

// Pick resolves token to one of list, which is newest first. Sessions reuse
// names over time, so a token that is exactly a name picks the newest session
// with that name. Otherwise the token must name one session, as
// target.Resolve decides.
func Pick(token string, list []Session) (Session, error) {
	for _, p := range list {
		if p.Name != "" && p.Name == token {
			return p, nil
		}
	}
	picked, err := target.Resolve([]string{token}, list)
	if err == nil {
		return picked[0], nil
	}
	if matches, merr := target.Match([]string{token}, list); merr == nil && (len(matches) == 1 || sameName(matches)) {
		return matches[0], nil
	}
	return Session{}, err
}

// sameName reports whether every session in list has the same, non-empty name.
func sameName(list []Session) bool {
	if len(list) == 0 || list[0].Name == "" {
		return false
	}
	for _, p := range list[1:] {
		if p.Name != list[0].Name {
			return false
		}
	}
	return true
}
