package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/paths"
	"github.com/uvw/claudes/internal/shell"
)

const (
	// stopWait bounds the wait for a stopped session's process to go, before
	// it is resumed: resuming a running session starts a copy of it.
	stopWait = 30 * time.Second
	// backWait bounds the wait for respawned sessions to show as running.
	backWait = 10 * time.Second
	// checkEvery is how often those waits look again.
	checkEvery = time.Second
)

// errTimedOut is what poll returns when its limit passes.
var errTimedOut = errors.New("timed out")

// poll calls check every checkEvery until it reports done, check fails, the
// limit passes (errTimedOut), or ctx ends (its error).
func (app *App) poll(ctx context.Context, limit time.Duration, check func() (done bool, err error)) error {
	deadline := app.Now().Add(limit)
	for {
		done, err := check()
		if err != nil || done {
			return err
		}
		if !app.Now().Before(deadline) {
			return errTimedOut
		}
		app.log().Debug("not yet; checking again", "in", checkEvery, "until", deadline.Format(time.TimeOnly))
		if err := app.Sleep(ctx, checkEvery); err != nil {
			return err
		}
	}
}

func (app *App) stopSession(ctx context.Context, s claude.Session) error {
	if !s.Running() {
		return errors.New("not running")
	}
	if s.Interactive() {
		return app.Terminate(s.PID, s.Started(), app.listedAt)
	}
	if s.JobID == "" {
		return errors.New("claude agents gave no job id for this session")
	}
	started, _ := app.Started(s.PID)
	if err := app.Agents.Stop(ctx, s.JobID); err != nil {
		return err
	}
	// claude stop returns while the process still runs, and a resume before
	// it exits starts a copy, even one the user runs right after claudes.
	err := app.poll(ctx, exitWait, func() (bool, error) { return !app.Alive(s.PID, started), nil })
	if errors.Is(err, errTimedOut) {
		return fmt.Errorf("claude stop returned, but process %d still runs after %s", s.PID, exitWait)
	}
	return err
}

// exitWait bounds the wait for a stopped session's process to exit. It takes
// about a second on Windows.
const exitWait = 15 * time.Second

// parkAndStop parks a session, then stops it. If the stop fails and the
// session is seen still running, it is unparked again.
func (app *App) parkAndStop(ctx context.Context, s claude.Session) error {
	if err := app.parkOne(s, false); err != nil {
		return err
	}
	if err := app.stopSession(ctx, s); err != nil {
		return app.stopFailed(ctx, s, err)
	}
	return nil
}

// stopHost stops the session running claudes, which ends this command, so it
// says so first. heading is the verb, such as "Stopping". An interactive host
// cannot bring itself back, so it is parked, unless it already is, and its
// command is printed.
func (app *App) stopHost(ctx context.Context, host claude.Session, heading string, alreadyParked bool) error {
	parked := alreadyParked
	if host.Interactive() && !parked {
		if err := app.parkOne(host, false); err != nil {
			return err
		}
		parked = true
	}
	if host.Interactive() {
		app.say("\n%s this session (%s) last. Output ends here. To bring it back, run claudes resume, or:", heading, host.Label())
		app.say("  %s", host.ManualCommand())
		app.record(func(rep *report) {
			e := entryOf(host)
			e.Command = host.ManualCommand()
			rep.Manual = append(rep.Manual, e)
		})
	} else {
		app.say("\n%s this session (%s) last. Output ends here.", heading, host.Label())
	}
	app.lastWords(host, strings.ToLower(heading))
	err := app.stopSession(ctx, host)
	if err != nil && parked {
		return app.stopFailed(ctx, host, err)
	}
	return err
}

// lastWords reports the session running claudes, then prints the report and
// frees the run lock, since stopping that session ends claudes.
func (app *App) lastWords(host claude.Session, verb string) {
	app.record(func(rep *report) {
		e := entryOf(host)
		e.Result, e.Note = verb, "this session, last: claudes ends when it stops"
		rep.Results = append(rep.Results, e)
	})
	app.printReport()
	app.unlock()
}

// resumeIndent lines up the notes claude prints under a progress line.
const resumeIndent = "             "

func (app *App) resumeSession(ctx context.Context, sessionID, cwd, prompt string) error {
	return app.resumeAs(ctx, sessionID, cwd, prompt, claude.NewJob{})
}

// resumeAs resumes a session with claude --bg --resume and waits for it to show
// as running. For a session with no background job, job gives the new job's
// name and options; resumeSession passes none, and Claude Code wakes a job
// with the options it started with.
func (app *App) resumeAs(ctx context.Context, sessionID, cwd, prompt string, job claude.NewJob) error {
	out, err := app.Agents.Resume(ctx, sessionID, cwd, prompt, job)
	if err != nil {
		return err
	}
	for line := range strings.Lines(out) {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "note:") {
			app.say("%s%s", resumeIndent, line)
		}
	}
	// claude agents lists a resumed session without a PID for about a second,
	// and a command run in that window would take it for stopped.
	err = app.poll(ctx, startWait, func() (bool, error) {
		sessions, err := app.Agents.Sessions(ctx, true)
		if err != nil {
			return false, err
		}
		return runningIn(sessions, func(x claude.Session) bool { return x.SessionID == sessionID }), nil
	})
	if errors.Is(err, errTimedOut) {
		return errNotYetRunning
	}
	return err
}

// errNotYetRunning is a resume claude accepted for a session that has not
// shown as running within startWait. It may still come up, so it is not
// resumed again: running claude --resume now would start a copy.
var errNotYetRunning = fmt.Errorf("claude accepted the resume, but the session does not show as running after %s. "+
	"Check claudes ls in a moment before resuming it again, since resuming a running session starts a copy", startWait)

// startWait bounds the wait for a resumed session to show as running.
const startWait = 15 * time.Second

// runningIn reports whether a session that match picks is running.
func runningIn(sessions []claude.Session, match func(claude.Session) bool) bool {
	return slices.ContainsFunc(sessions, func(s claude.Session) bool { return match(s) && s.Running() })
}

// waitGone waits until a stopped session's process has gone.
// claude agents drops the PID as soon as claude stop returns, but the
// process lives on for about a second, and a resume until then starts a copy.
// So this waits for the process too.
func (app *App) waitGone(ctx context.Context, s claude.Session) error {
	err := app.poll(ctx, stopWait, func() (bool, error) {
		sessions, err := app.Agents.Sessions(ctx, true)
		if err != nil {
			return false, err
		}
		return !runningIn(sessions, func(x claude.Session) bool { return x.SessionID == s.SessionID }), nil
	})
	if errors.Is(err, errTimedOut) {
		return fmt.Errorf("still running %s after it was stopped", stopWait)
	}
	return err
}

// checkBack waits for respawned sessions to show as running, and returns a
// failure for each one that does not.
func (app *App) checkBack(ctx context.Context, respawned []claude.Session) []failure {
	if len(respawned) == 0 {
		return nil
	}
	var missing []claude.Session
	err := app.poll(ctx, backWait, func() (bool, error) {
		sessions, err := app.Agents.Sessions(ctx, true)
		if err != nil {
			return false, err
		}
		missing = slices.DeleteFunc(slices.Clone(respawned), func(s claude.Session) bool {
			// The old process can still show for a moment after respawn
			// returns, so only a new PID counts as back.
			return runningIn(sessions, func(x claude.Session) bool { return x.JobID == s.JobID && x.PID != s.PID })
		})
		return len(missing) == 0, nil
	})
	reason := ""
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errTimedOut):
		reason = fmt.Sprintf("respawned, but not running after %s: check claudes ls", backWait)
	case ctx.Err() != nil:
		reason = "respawned, but claudes was interrupted before it saw it running: check claudes ls"
	default:
		return []failure{{label: "restarted sessions", reason: "cannot check they came back: " + err.Error()}}
	}
	out := make([]failure, 0, len(missing))
	for _, s := range missing {
		out = append(out, failure{s.Label(), reason, s})
	}
	return out
}

// parkOne records a session in the park file before it is stopped, so a crash
// or a failed resume leaves it for claudes resume.
func (app *App) parkOne(s claude.Session, wasBusy bool) error {
	e := park.FromSession(s, wasBusy, app.Now().UTC().Format(time.RFC3339))
	return app.Park.Update(func(entries []park.Entry) []park.Entry { return park.Merge(entries, []park.Entry{e}) })
}

func (app *App) unpark(sessionID string) error {
	return app.Park.Update(func(entries []park.Entry) []park.Entry {
		return slices.DeleteFunc(entries, func(e park.Entry) bool { return e.SessionID == sessionID })
	})
}

// relaunch stops a session and resumes it in the background with prompt. The
// session is parked until it is back, and is not stopped at all when its
// directory is gone, because it could not be resumed.
func (app *App) relaunch(ctx context.Context, s claude.Session, prompt string) error {
	if !paths.IsDir(s.Cwd) {
		return fmt.Errorf("directory %s is gone, so it was not stopped", s.Cwd)
	}
	if err := app.parkOne(s, prompt != ""); err != nil {
		return err
	}
	if err := app.stopSession(ctx, s); err != nil {
		return app.stopFailed(ctx, s, err)
	}
	parked := fmt.Sprintf("Still parked: run claudes resume %s, or: %s", shell.Arg(s.ID()), s.ManualCommand())
	if err := app.waitGone(ctx, s); err != nil {
		return fmt.Errorf("stopped, but not resumed (%w). %s", err, parked)
	}
	if err := app.resumeSession(ctx, s.SessionID, s.Cwd, prompt); errors.Is(err, errNotYetRunning) {
		// Kept parked, so claudes resume brings it back if it never came up.
		app.warn("claudes: %s: %v.", s.Label(), err)
		return nil
	} else if err != nil {
		return fmt.Errorf("stopped, but not resumed (%w). %s", err, parked)
	}
	return app.unpark(s.SessionID)
}

// stopSettle bounds how long stopFailed watches for a stop that lands late.
const stopSettle = 5 * time.Second

// stopFailed handles a stop that returned an error. The stop may still take
// effect, such as when `claude stop` times out after the job got the signal,
// so the session is unparked only if it is still running after stopSettle.
func (app *App) stopFailed(ctx context.Context, s claude.Session, stopErr error) error {
	hand := context.WithoutCancel(ctx)
	err := app.poll(hand, stopSettle, func() (bool, error) {
		sessions, err := app.Agents.Sessions(hand, true)
		if err != nil {
			return false, err
		}
		return !runningIn(sessions, func(x claude.Session) bool { return x.SessionID == s.SessionID }), nil
	})
	if errors.Is(err, errTimedOut) {
		if err := app.unpark(s.SessionID); err != nil {
			return errors.Join(stopErr, err)
		}
		return stopErr
	}
	return fmt.Errorf("%w. It may be stopped: it stays parked for claudes resume", stopErr)
}

// freshFor is how old a listing may be when claudes acts on a session from
// it. A session can start a turn between the list and the action, and its
// status decides whether it is pinged.
const freshFor = 5 * time.Second

// current is s as claude agents lists it now, listing again when the last
// listing is older than freshFor, and whether it is still running.
func (app *App) current(ctx context.Context, s claude.Session) (claude.Session, bool, error) {
	if app.lastList == nil || app.Now().Sub(app.listedAt) >= freshFor {
		if _, err := app.listed(ctx, true); err != nil {
			return s, false, err
		}
	}
	for _, x := range app.lastList {
		if x.SessionID == s.SessionID {
			return x, x.Running(), nil
		}
	}
	return s, false, nil
}

// notReached names the sessions a Ctrl-C left untouched.
func notReached[T subject](app *App, sessions []T) {
	if len(sessions) > 0 {
		app.warn("\nNot reached after Ctrl-C, left as they were: %s", labels(sessions))
	}
	for _, s := range sessions {
		app.record(func(rep *report) { rep.NotReached = append(rep.NotReached, entryOf(s)) })
	}
}

// countParked is how many of sessions the park file holds.
func (app *App) countParked(sessions []claude.Session) int {
	entries, err := app.Park.Load()
	if err != nil {
		return 0
	}
	n := 0
	for _, s := range sessions {
		if slices.ContainsFunc(entries, func(e park.Entry) bool { return e.SessionID == s.SessionID }) {
			n++
		}
	}
	return n
}
