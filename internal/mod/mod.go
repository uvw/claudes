// Package mod is the request channel to claudes-mod, the Claude Code plugin in
// plugins/claudes-mod that runs inside each session. claudes writes one
// request file per session, the mod runs the commands it allows once the
// session is idle, and writes a result file for each. The mod also writes a
// presence file when a session loads it.
//
// The files live under claudes' state directory, in a directory only the user
// can read.
package mod

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/safefile"
)

const (
	// Plugin and Marketplace name the mod in Claude Code: the plugin's name
	// in plugins/claudes-mod and the marketplace's in .claude-plugin.
	Plugin      = "claudes-mod"
	Marketplace = "claudes"
	// ID is how claude plugin commands name the mod.
	ID = Plugin + "@" + Marketplace
	// DefaultSource is the marketplace a released claudes installs from.
	DefaultSource = "uvw/claudes"
	// Protocol is the version of the request format. The mod refuses a
	// request for any other.
	Protocol = 1
)

// The statuses of a result.
const (
	Done    = "done"
	Refused = "refused"
	Error   = "error"
)

// Request is one command for the mod to run, as `/COMMAND ARGS`.
type Request struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Args    string `json:"args"`
}

// NewRequest is a request with a fresh id.
func NewRequest(command, args string) Request {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return Request{ID: hex.EncodeToString(b), Command: command, Args: args}
}

type requestFile struct {
	Protocol int    `json:"protocol"`
	Session  string `json:"session"`
	// Expires is in milliseconds since the Unix epoch. The mod refuses a
	// request it finds later, so one claudes left behind never runs.
	Expires  int64     `json:"expires"`
	Requests []Request `json:"requests"`
}

// Result is what the mod did with one request.
type Result struct {
	ID      string `json:"id"`
	Session string `json:"session"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	// At is in milliseconds since the Unix epoch.
	At      int64  `json:"at"`
	Version string `json:"version"`
}

// Presence is what the mod writes when a session loads it.
type Presence struct {
	Protocol int    `json:"protocol"`
	Session  string `json:"session"`
	Version  string `json:"version"`
	// At is in milliseconds since the Unix epoch.
	At int64 `json:"at"`
}

// Time is when the mod loaded.
func (p Presence) Time() time.Time { return time.UnixMilli(p.At) }

// Channel is the directory claudes and the mod share.
type Channel struct{ Dir string }

// DefaultChannel is the mod directory under claudes' state directory.
func DefaultChannel() (Channel, error) {
	state, err := paths.State()
	if err != nil {
		return Channel{}, err
	}
	return Channel{Dir: filepath.Join(state, "mod")}, nil
}

func (c Channel) requests() string { return filepath.Join(c.Dir, "requests") }
func (c Channel) results() string  { return filepath.Join(c.Dir, "results") }
func (c Channel) sessions() string { return filepath.Join(c.Dir, "sessions") }

// Prepare creates the channel's directories for the user alone. The mod may
// have created them first, with the default mode, so it sets the mode too.
func (c Channel) Prepare() error {
	for _, d := range []string{c.Dir, c.requests(), c.results(), c.sessions()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(d, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func checkSession(sessionID string) error {
	if !claude.ValidSessionID(sessionID) {
		return fmt.Errorf("%q is not a session id", sessionID)
	}
	return nil
}

// Send replaces sessionID's request file with reqs.
func (c Channel) Send(sessionID string, reqs []Request, expires time.Time) error {
	if err := checkSession(sessionID); err != nil {
		return err
	}
	if err := c.Prepare(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(requestFile{Protocol: Protocol, Session: sessionID, Expires: expires.UnixMilli(), Requests: reqs}, "", "  ")
	if err != nil {
		return err
	}
	return safefile.WriteFile(filepath.Join(c.requests(), sessionID+".json"), append(data, '\n'), 0o600)
}

// Results reads the results the mod has written for reqs so far, by id.
func (c Channel) Results(reqs []Request) (map[string]Result, error) {
	out := map[string]Result{}
	for _, r := range reqs {
		var res Result
		ok, err := readJSON(filepath.Join(c.results(), r.ID+".json"), &res)
		if err != nil {
			return out, err
		}
		if ok {
			out[r.ID] = res
		}
	}
	return out, nil
}

// Clear removes sessionID's request file and the results of reqs.
func (c Channel) Clear(sessionID string, reqs []Request) error {
	if err := checkSession(sessionID); err != nil {
		return err
	}
	errs := []error{removeIfThere(filepath.Join(c.requests(), sessionID+".json"))}
	for _, r := range reqs {
		errs = append(errs, removeIfThere(filepath.Join(c.results(), r.ID+".json")))
	}
	return errors.Join(errs...)
}

// Presence reads what the mod wrote when sessionID loaded it, and whether it
// did.
func (c Channel) Presence(sessionID string) (Presence, bool, error) {
	if err := checkSession(sessionID); err != nil {
		return Presence{}, false, err
	}
	var p Presence
	ok, err := readJSON(filepath.Join(c.sessions(), sessionID+".json"), &p)
	return p, ok, err
}

// Remove deletes the channel and everything in it.
func (c Channel) Remove() error { return os.RemoveAll(c.Dir) }

// readJSON reads a file the mod wrote. The mod cannot write a file
// atomically, so one that does not parse yet counts as not there.
func readJSON(path string, v any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return json.Unmarshal(data, v) == nil, nil
}

func removeIfThere(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
