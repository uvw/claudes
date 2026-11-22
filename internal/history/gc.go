package history

import (
	"cmp"
	"path/filepath"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/transcripts"
)

// Floor is how recently a session or directory may have changed and still be
// picked without being named: one that changed since may be starting.
const Floor = 24 * time.Hour

// Kind is why session gc removes an item.
type Kind string

const (
	KindEmpty    Kind = "empty"
	KindTrivial  Kind = "trivial"
	KindHeadless Kind = "headless"
	KindGone     Kind = "gone"
	KindOld      Kind = "old"
	KindCopy     Kind = "copy"
	KindSidecar  Kind = "sidecar"
	KindLeftover Kind = "leftover"
	KindNamed    Kind = "named"
)

// Item is what session gc removes in one go: a whole session, an older copy
// of one, or a leftover directory.
type Item struct {
	SessionID, Name string
	Why             Kind
	Paths           []string
	Size            int64
	// Transcript and Changed are the transcript the item was planned from and
	// when it last changed then, so a session that came back to life since is
	// left alone. A leftover has neither.
	Transcript string
	Changed    time.Time
	// Copies are the transcripts the item removes, whose records session mv
	// may have kept: every copy of a whole session, or the one older copy.
	Copies []string
	dir    string
}

// ID is the session's short id for a whole session, and "" for a part of one:
// session gc ID removes the whole session, so an older copy, a sidecar, or a
// leftover must not offer it.
func (g Item) ID() string {
	switch g.Why {
	case KindCopy, KindSidecar, KindLeftover:
		return ""
	}
	return claude.ShortID(g.SessionID)
}

func (g Item) Label() string { return g.Name }

// Dir is where the item is: its session's directory, or its own.
func (g Item) Dir() string { return g.dir }

// Kinds are the kinds of item session gc picks without targets.
type Kinds struct {
	Empty, Trivial, Headless, Gone, Copies, Leftovers, Sidecars bool
	// OlderThan takes sessions last active longer ago than this, or trims
	// their subagent transcripts with Sidecars; 0 takes none.
	OlderThan time.Duration
}

// Keep is the sessions session gc must leave alone, each with why: running,
// listed by agent view, parked, or the session running claudes. listed is
// what agent view lists, parked the session ids the park file holds, and
// isHost picks the session running claudes by its id.
func Keep(list []Session, listed []claude.Session, parked []string, isHost func(sessionID string) bool) map[string]string {
	keep := map[string]string{}
	for _, p := range list {
		if p.Running {
			keep[p.SessionID] = "is running"
		}
	}
	for _, s := range listed {
		switch {
		case s.Running():
			keep[s.SessionID] = "is running"
		case keep[s.SessionID] == "":
			keep[s.SessionID] = "is still in agent view's list (claudes gc removes it from the list)"
		}
	}
	for _, id := range parked {
		keep[id] = "is parked"
	}
	for _, p := range list {
		if isHost(p.SessionID) {
			keep[p.SessionID] = "is the session running claudes"
		}
	}
	return keep
}

// Collect finds what session gc removes without targets: the items of kinds
// in list, except the sessions in keep and anything that changed since Floor
// before now. With Trivial or Headless, it reads the activity of the sessions
// it weighs into list.
func Collect(st transcripts.Store, list []Session, keep map[string]string, kinds Kinds, now time.Time) ([]Item, error) {
	recent := now.Add(-Floor)
	var items []Item
	for i := range list {
		p := &list[i]
		if _, kept := keep[p.SessionID]; kept || p.LastActive.After(recent) {
			continue
		}
		if (kinds.Trivial || kinds.Headless) && !p.Empty {
			p.ReadTally(false)
		}
		whole := len(items)
		switch {
		case kinds.Empty && p.Empty:
			items = append(items, Whole(st, *p, KindEmpty))
		case kinds.Trivial && trivial(*p):
			items = append(items, Whole(st, *p, KindTrivial))
		case kinds.Headless && headless(*p):
			items = append(items, Whole(st, *p, KindHeadless))
		case kinds.Gone && paths.Gone(p.Cwd):
			items = append(items, Whole(st, *p, KindGone))
		case kinds.OlderThan > 0 && !kinds.Sidecars && p.LastActive.Before(now.Add(-kinds.OlderThan)):
			items = append(items, Whole(st, *p, KindOld))
		case kinds.Copies:
			for _, c := range p.Copies {
				if changed := transcripts.Changed(c); !changed.IsZero() && changed.Before(recent) {
					paths := transcripts.CopyFiles(c)
					items = append(items, Item{SessionID: p.SessionID, Name: p.Label() + " (older copy)", Why: KindCopy,
						dir: filepath.Dir(c), Paths: paths, Size: transcripts.Size(paths), Transcript: c, Changed: changed, Copies: []string{c}})
				}
			}
		}
		removed := len(items) > whole && items[whole].Why != KindCopy
		if kinds.Sidecars && !removed {
			old := kinds.OlderThan > 0 && p.LastActive.Before(now.Add(-kinds.OlderThan))
			if it, ok := sidecarItem(*p, old); ok {
				items = append(items, it)
			}
		}
	}
	if !kinds.Leftovers {
		return items, nil
	}
	sessions := make([]transcripts.Session, len(list))
	for i, p := range list {
		sessions[i] = p.Session
	}
	leftovers, err := st.Leftovers(sessions)
	if err != nil {
		return nil, err
	}
	for _, l := range leftovers {
		if _, kept := keep[l.SessionID]; kept || l.Modified.After(recent) {
			continue
		}
		items = append(items, Item{SessionID: l.SessionID, Name: l.Kind + " " + claude.ShortID(l.SessionID),
			Why: KindLeftover, dir: l.Path, Paths: []string{l.Path}, Size: l.Size})
	}
	return items, nil
}

// Whole is p as an item that removes the whole session, with every file
// Claude Code keeps for it in st.
func Whole(st transcripts.Store, p Session, why Kind) Item {
	return Item{SessionID: p.SessionID, Name: p.Label(), Why: why, dir: cmp.Or(p.Cwd, filepath.Dir(p.Path)),
		Paths: st.SessionFiles(p.Session), Size: p.Disk.Total + st.ExtraSize(p.Session),
		Transcript: p.Path, Changed: transcripts.Changed(p.Path), Copies: append([]string{p.Path}, p.Copies...)}
}

// sidecarItem is the part of p's sidecar directory --sidecars trims: its saved
// command outputs, and its subagent transcripts too when withSubagents.
func sidecarItem(p Session, withSubagents bool) (Item, bool) {
	parts := transcripts.SidecarParts(p.Session, withSubagents)
	if len(parts) == 0 {
		return Item{}, false
	}
	it := Item{SessionID: p.SessionID, Why: KindSidecar, dir: cmp.Or(p.Cwd, transcripts.Sidecar(p.Session)),
		Transcript: p.Path, Changed: transcripts.Changed(p.Path)}
	what := make([]string, len(parts))
	for i, part := range parts {
		it.Paths, it.Size, what[i] = append(it.Paths, part.Path), it.Size+part.Size, part.What
	}
	it.Name = p.Label() + " (" + strings.Join(what, " and ") + ")"
	return it, true
}

// trivial reports whether p holds nothing worth keeping: it changed no file,
// made no tool call, was not named with /rename or started by an agent team,
// and either only ran commands or got exactly one prompt. A transcript claudes
// could not read all of is never trivial.
func trivial(p Session) bool {
	a := p.Activity
	if a == nil || !a.Complete() || p.Empty || a.Edits > 0 || a.ToolCalls > 0 || a.Teammate || p.NameSource == transcripts.NameCustom || p.Disk.FileHistory > 0 {
		return false
	}
	return a.Prompts == 1 || a.Prompts == 0 && a.Commands > 0
}

// headless reports whether p is a one-shot a program ran: claude -p or the
// Agent SDK, never opened in a terminal, with one prompt and no file changed.
// Its answer went to the program that asked, so the transcript holds nothing
// to resume. Tool calls are allowed, such as the git diff a commit message
// tool runs; edits, a name from /rename, and an agent team are not.
func headless(p Session) bool {
	a := p.Activity
	if a == nil || !a.Complete() || p.Empty || !a.Headless || a.Edits > 0 || a.Teammate || p.NameSource == transcripts.NameCustom || p.Disk.FileHistory > 0 {
		return false
	}
	return a.Prompts == 1
}
