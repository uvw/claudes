package history

import (
	"time"

	"github.com/uvw/claudes/internal/transcripts"
)

// Hit is a session search found, as -o json prints it.
type Hit struct {
	ID         string              `json:"id"`
	SessionID  string              `json:"sessionId"`
	Name       string              `json:"name"`
	Cwd        string              `json:"cwd"`
	Score      float64             `json:"score"`
	LastActive time.Time           `json:"lastActive"`
	Matches    []transcripts.Match `json:"matches"`
}

// Hits are results as session search shows them, with the secrets in each
// name masked unless raw.
func Hits(results []transcripts.Result, raw bool) []Hit {
	hits := make([]Hit, len(results))
	for i, r := range results {
		name := r.Name
		if !raw {
			name = transcripts.Redact(name)
		}
		hits[i] = Hit{ID: r.ID(), SessionID: r.SessionID, Name: name, Cwd: r.Cwd, Score: r.Score, LastActive: r.LastActive, Matches: r.Matches}
	}
	return hits
}
