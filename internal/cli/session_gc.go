package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
	"github.com/uvw/claudes/internal/target"
	"github.com/uvw/claudes/internal/transcripts"
)

// gcFloor is how recently a session or folder may have changed and still be
// picked without being named: one that changed since may be starting.
const gcFloor = 24 * time.Hour

// gcItem is what session gc removes in one go: a whole session, an older copy
// of one, or a leftover folder.
type gcItem struct {
	sessionID, name string
	// why is empty, trivial, gone, old, copy, sidecar, leftover, or named.
	why    string
	folder string
	paths  []string
	size   int64
	// transcript and changed are the transcript the item was planned from and
	// when it last changed then, so a session that came back to life since is
	// left alone. A leftover has neither.
	transcript string
	changed    time.Time
}

func (g gcItem) ID() string    { return claude.ShortID(g.sessionID) }
func (g gcItem) Label() string { return g.name }

type gcFlags struct {
	olderThan                                         days
	empty, trivial, gone, copies, leftovers, sidecars bool
	dryRun, purge                                     bool
}

func (app *App) sessionGcCommand() *cobra.Command {
	var f gcFlags
	cmd := &cobra.Command{
		Use:   "gc [TARGET...]",
		Short: "Remove past sessions that are empty, gone, old, or left over",
		Long: `Remove past sessions from ~/.claude, with everything Claude Code keeps for each:
its transcript, subagent transcripts and tool results, older copies, file
history, session-env folder, and debug log. They go to the system trash, so a
mistake can be undone; --purge deletes them for good.

Without flags or targets, removes:

  empty      sessions that never got a prompt
  copies     older copies session mv left, keeping the newest
  leftovers  subagent and file-history folders whose transcript is gone

--empty, --trivial, --gone, --copies, --leftovers, and --older-than pick
exactly the kinds they name instead. --trivial takes sessions that changed no
file and made no tool call, and either only ran slash commands or got one
prompt, such as a quick health check; a session renamed with /rename or started
by an agent team is never trivial. --gone takes sessions whose folder no longer
exists, which includes a folder on a disk that is not attached, so read the
plan first. --older-than takes sessions last active longer ago than that.

--sidecars keeps each session and trims its sidecar folder instead: the big
command outputs Claude Code saved to tool-results, which the transcript keeps a
preview of. With --older-than, it also takes the subagent transcripts of
sessions last active longer ago than that, and removes no whole session. A
session resumes as before; it only cannot read a saved output again, or
continue a subagent it ran.

It never removes a running session, one agent view still lists (claudes gc
removes those from the list), a parked one, or the session running claudes.
Without targets, it also leaves alone anything that changed in the last day.
Targets remove exactly those sessions, whatever kind they are.`,
		ValidArgsFunction: app.completePast(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.sessionGc(cmd.Context(), args, f)
		},
	}
	cmd.Flags().BoolVar(&f.empty, "empty", false, "sessions that never got a prompt")
	cmd.Flags().BoolVar(&f.trivial, "trivial", false, "sessions that only ran slash commands, or got one prompt and made no tool call")
	cmd.Flags().BoolVar(&f.gone, "gone", false, "sessions whose folder no longer exists")
	cmd.Flags().BoolVar(&f.copies, "copies", false, "older copies session mv left")
	cmd.Flags().BoolVar(&f.sidecars, "sidecars", false, "saved command outputs, and with --older-than subagent transcripts, keeping the sessions")
	cmd.Flags().BoolVar(&f.leftovers, "leftovers", false, "subagent and file-history folders whose transcript is gone")
	cmd.Flags().Var(&f.olderThan, "older-than", "sessions last active longer ago than this, such as 90d")
	cmd.Flags().Lookup("older-than").DefValue = ""
	cmd.Flags().BoolVar(&f.purge, "purge", false, "delete for good instead of moving to the trash")
	addDryRun(cmd, &f.dryRun)
	return cmd
}

func (app *App) sessionGc(ctx context.Context, targets []string, f gcFlags) error {
	if len(targets) > 0 && (f.empty || f.trivial || f.gone || f.copies || f.leftovers || f.sidecars || f.olderThan.d > 0) {
		return errors.New("session gc takes targets or kinds to remove, not both")
	}
	if !f.empty && !f.trivial && !f.gone && !f.copies && !f.leftovers && !f.sidecars && f.olderThan.d == 0 {
		f.empty, f.copies, f.leftovers = true, true, true
	}
	list, err := app.pastSessions(ctx)
	if err != nil {
		return err
	}
	if !app.runningKnown {
		return refused(errors.New("cannot tell which sessions are running, so session gc removes nothing"))
	}
	keep, err := app.gcKeep(ctx, list)
	if err != nil {
		return err
	}

	var items []gcItem
	if len(targets) > 0 {
		picked, err := target.Resolve(targets, list)
		if err != nil {
			return err
		}
		for _, p := range picked {
			if why, ok := keep[p.SessionID]; ok {
				return refused(fmt.Errorf("%s %s, so session gc leaves it alone", p.Label(), why))
			}
			items = append(items, app.wholeSession(p, "named"))
		}
	} else {
		if items, err = app.gcPick(list, keep, f); err != nil {
			return err
		}
	}
	if len(items) == 0 {
		app.say("Nothing to clean up.")
		return nil
	}

	var total int64
	rows := make([]planRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, planRow{it.ID(), it.name, it.why, it.folder, style.Size(it.size)})
		total += it.size
	}
	app.printPlanOf("Removing", "session(s) and folder(s)", rows)
	where := "Moves them to the trash; --purge deletes them for good."
	if f.purge {
		where = "Deletes them for good."
	}
	app.say("\nFrees %s. %s", style.Size(total), where)
	if f.dryRun {
		app.say("\nDry run: nothing was changed.")
		return nil
	}
	return app.gcRemove(ctx, items, f.purge)
}

// gcKeep is the sessions session gc must leave alone, each with why: running,
// listed by agent view, parked, or the session running claudes.
func (app *App) gcKeep(ctx context.Context, list []pastSession) (map[string]string, error) {
	keep := map[string]string{}
	for _, p := range list {
		if p.Running {
			keep[p.SessionID] = "is running"
		}
	}
	listed, err := app.Agents.Sessions(ctx, true)
	if err != nil {
		return nil, refused(fmt.Errorf("cannot list agent view's sessions, so session gc removes nothing: %w", err))
	}
	for _, s := range listed {
		switch {
		case s.Running():
			keep[s.SessionID] = "is running"
		case keep[s.SessionID] == "":
			keep[s.SessionID] = "is still in agent view's list (claudes gc removes it from the list)"
		}
	}
	parked, err := app.Park.Load()
	if err != nil {
		return nil, fmt.Errorf("cannot read the park file, so cannot tell which sessions are paused: %w", err)
	}
	for _, e := range parked {
		keep[e.SessionID] = "is parked"
	}
	for _, p := range list {
		if app.isHostID(p.SessionID) {
			keep[p.SessionID] = "is the session running claudes"
		}
	}
	return keep, nil
}

// gcPick finds what session gc removes without targets.
func (app *App) gcPick(list []pastSession, keep map[string]string, f gcFlags) ([]gcItem, error) {
	now := app.Now()
	recent := now.Add(-gcFloor)
	var items []gcItem
	for _, p := range list {
		if _, kept := keep[p.SessionID]; kept || p.LastActive.After(recent) {
			continue
		}
		if f.trivial && !p.Empty {
			readActivity(&p, false)
		}
		whole := len(items)
		switch {
		case f.empty && p.Empty:
			items = append(items, app.wholeSession(p, "empty"))
		case f.trivial && trivial(p):
			items = append(items, app.wholeSession(p, "trivial"))
		case f.gone && folderGone(p.Cwd):
			items = append(items, app.wholeSession(p, "gone"))
		case f.olderThan.d > 0 && !f.sidecars && p.LastActive.Before(now.Add(-f.olderThan.d)):
			items = append(items, app.wholeSession(p, "old"))
		case f.copies:
			for _, c := range p.Copies {
				if changed := modTime(c); !changed.IsZero() && changed.Before(recent) {
					paths := transcripts.CopyFiles(c)
					items = append(items, gcItem{sessionID: p.SessionID, name: p.Label() + " (older copy)", why: "copy",
						folder: filepath.Dir(c), paths: paths, size: sizeOf(paths), transcript: c, changed: changed})
				}
			}
		}
		removed := len(items) > whole && items[whole].why != "copy"
		if f.sidecars && !removed {
			old := f.olderThan.d > 0 && p.LastActive.Before(now.Add(-f.olderThan.d))
			if it, ok := sidecarItem(p, old); ok {
				items = append(items, it)
			}
		}
	}
	if !f.leftovers {
		return items, nil
	}
	sessions := make([]transcripts.Session, len(list))
	for i, p := range list {
		sessions[i] = p.Session
	}
	leftovers, err := app.Transcripts.Leftovers(sessions)
	if err != nil {
		return nil, err
	}
	for _, l := range leftovers {
		if _, kept := keep[l.SessionID]; kept || l.Modified.After(recent) {
			continue
		}
		items = append(items, gcItem{sessionID: l.SessionID, name: l.Kind + " " + claude.ShortID(l.SessionID),
			why: "leftover", folder: l.Path, paths: []string{l.Path}, size: l.Size})
	}
	return items, nil
}

// sidecarItem is the part of p's sidecar folder --sidecars trims: its saved
// command outputs, and its subagent transcripts too when withSubagents.
func sidecarItem(p pastSession, withSubagents bool) (gcItem, bool) {
	sidecar := transcripts.Sidecar(p.Session)
	parts := []struct{ folder, what string }{{"tool-results", "saved outputs"}}
	if withSubagents {
		parts = append(parts, struct{ folder, what string }{"subagents", "subagents"})
	}
	var paths, what []string
	for _, part := range parts {
		path := filepath.Join(sidecar, part.folder)
		if info, err := os.Lstat(path); err == nil && info.IsDir() && sizeOf([]string{path}) > 0 {
			paths, what = append(paths, path), append(what, part.what)
		}
	}
	if len(paths) == 0 {
		return gcItem{}, false
	}
	return gcItem{sessionID: p.SessionID, name: p.Label() + " (" + strings.Join(what, " and ") + ")", why: "sidecar", folder: cmp.Or(p.Cwd, sidecar),
		paths: paths, size: sizeOf(paths), transcript: p.Path, changed: modTime(p.Path)}, true
}

// trivial reports whether p holds nothing worth keeping: it changed no file,
// made no tool call, was not named with /rename or started by an agent team,
// and either only ran commands or got exactly one prompt.
func trivial(p pastSession) bool {
	a := p.Activity
	if a == nil || p.Empty || a.Edits > 0 || a.ToolCalls > 0 || a.Teammate || p.NameSource == "custom" || p.Disk.FileHistory > 0 {
		return false
	}
	return a.Prompts == 1 || a.Prompts == 0 && a.Commands > 0
}

func (app *App) wholeSession(p pastSession, why string) gcItem {
	return gcItem{sessionID: p.SessionID, name: p.Label(), why: why, folder: cmp.Or(p.Cwd, filepath.Dir(p.Path)),
		paths: app.Transcripts.SessionFiles(p.Session), size: p.Disk.Total + app.extraSize(p.Session),
		transcript: p.Path, changed: modTime(p.Path)}
}

// extraSize is what SessionFiles takes beyond the session's Disk: its
// session-env folder and debug log.
func (app *App) extraSize(s transcripts.Session) int64 {
	if app.Transcripts.Home == "" {
		return 0
	}
	return sizeOf([]string{
		filepath.Join(app.Transcripts.Home, "session-env", s.SessionID),
		filepath.Join(app.Transcripts.Home, "debug", s.SessionID+".txt"),
	})
}

// gcRemove removes items, after checking once more that none of them runs
// and that none changed since the plan.
func (app *App) gcRemove(ctx context.Context, items []gcItem, purge bool) error {
	running, err := app.Agents.Sessions(ctx, false)
	if err != nil {
		return refused(fmt.Errorf("cannot tell which sessions are running, so session gc removes nothing: %w", err))
	}
	live := map[string]bool{}
	for _, s := range running {
		if s.Running() {
			live[s.SessionID] = true
		}
	}
	root := cmp.Or(app.Transcripts.Home, app.Transcripts.Dir)
	verb := "trashed"
	if purge {
		verb = "deleted"
	}
	var failures []failure
	var freed int64
	for i, it := range items {
		if ctx.Err() != nil {
			notReached(app, items[i:])
			break
		}
		if live[it.sessionID] && it.why != "leftover" {
			app.progress("skipped", it, " (it started running)")
			continue
		}
		if it.transcript != "" && !modTime(it.transcript).Equal(it.changed) {
			app.progress("skipped", it, " (it changed since the plan)")
			continue
		}
		if err := app.gcRemoveItem(it, root, purge); err != nil {
			failures = append(failures, failure{it.name, err.Error(), it})
			app.progressFailed(it)
			continue
		}
		if it.why == "copy" {
			if err := app.Transcripts.ForgetMove(it.transcript, it.sessionID); err != nil {
				app.warn("claudes: removed %s, but cannot update moves.json: %v", it.transcript, err)
			}
		}
		freed += it.size
		app.progress(verb, it, " ("+style.Size(it.size)+")")
	}
	if freed > 0 {
		app.say("\nFreed %s.", style.Size(freed))
	}
	return app.reportFailures(failures)
}

// gcRemoveItem removes each of an item's paths. It refuses a path outside
// root, or one whose name, or whose folder's name, does not carry the session
// id, so nothing else is ever touched.
func (app *App) gcRemoveItem(it gcItem, root string, purge bool) error {
	for _, p := range it.paths {
		rel, err := filepath.Rel(root, p)
		owned := strings.Contains(filepath.Base(p), it.sessionID) || filepath.Base(filepath.Dir(p)) == it.sessionID
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") || !owned {
			return fmt.Errorf("refused to remove %s, which is not a file of session %s under %s", p, it.sessionID, root)
		}
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if purge {
			err = os.RemoveAll(p)
		} else {
			err = app.Trash(p)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// folderGone reports whether dir is known and no longer exists. A folder that
// cannot be checked, such as one behind a permission error, does not count.
func folderGone(dir string) bool {
	if dir == "" {
		return false
	}
	_, err := os.Stat(dir)
	return errors.Is(err, fs.ErrNotExist)
}

func modTime(path string) time.Time {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func sizeOf(paths []string) int64 {
	var n int64
	for _, p := range paths {
		_ = filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					n += info.Size()
				}
			}
			return nil
		})
	}
	return n
}
