package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/park"
	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
)

func (app *App) sessionMvCommand() *cobra.Command {
	var resume, rm bool
	cmd := &cobra.Command{
		Use:     "mv TARGET DIR",
		Aliases: []string{"move"},
		Short:   "Copy a session to another directory, so it resumes from there",
		Long: `Copy a session's transcript into DIR's project directory, so claude --resume
finds it when started there. The original stays, so the session still resumes
from its old directory too; claudes prints the command that removes it. The old
directory's project memory and the directory recorded inside the transcript are left
alone. --rm deletes the original once the copy matches it byte for byte.

A running session is refused, since what it says after the copy stays in the
old transcript. The session running claudes is the exception: copy it, then
exit it and resume it from DIR. --resume resumes the copy in this terminal.

A stopped background session's job would wake in the old directory, so mv
removes the job with claude rm and keeps the options Claude Code saved for it.
claudes session resume --bg then starts a new job in DIR with those options.`,
		Args: cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
			return app.completePast(true)(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.moveSession(cmd.Context(), args[0], args[1], resume, rm)
		},
	}
	cmd.Flags().BoolVarP(&resume, "resume", "r", false, "resume the session from DIR once it is copied")
	cmd.Flags().BoolVar(&rm, "rm", false, "delete the original once the copy matches it")
	app.addForce(cmd, "copy although claudes cannot tell whether the session runs, or Claude Code may name project directories differently now")
	return cmd
}

func (app *App) moveSession(ctx context.Context, token, folder string, resume, rm bool) error {
	dir, err := realFolder(folder)
	if err != nil {
		return err
	}
	list, err := app.pastSessions(ctx)
	if err != nil {
		return err
	}
	p, err := pickPast(token, list)
	if err != nil {
		return err
	}
	host := app.isHostID(p.SessionID)
	switch {
	case p.Running && !host:
		return refused(fmt.Errorf("%s is running, and what it says after the copy would stay behind. Stop it first", p.Label()))
	case !app.runningKnown && !host && !app.force:
		return refused(fmt.Errorf("cannot tell whether %s is running, and what a running session says after the copy stays behind. Try again, or pass --force", p.Label()))
	case host && resume:
		return fmt.Errorf("%s is the session running claudes, so it cannot resume here. Drop --resume, then exit it and run the printed command", p.Label())
	case host && rm:
		return fmt.Errorf("%s is the session running claudes, which writes to its transcript until it exits. Drop --rm, then exit it and run the printed remove command", p.Label())
	}
	sessions := make([]transcripts.Session, len(list))
	for i, s := range list {
		sessions[i] = s.Session
	}
	if !transcripts.FolderNamesMatch(sessions) {
		if err := app.refuse(app.Transcripts.Dir, []string{"recent sessions live in directories claudes would not name that way"}); err != nil {
			return err
		}
	}
	if sameFolder(p.Cwd, dir) {
		return fmt.Errorf("%s already lives under %s", p.Label(), dir)
	}
	job := movingJob{options: p.JobOptions()}
	if !host && app.runningKnown {
		if job, err = app.jobToMove(ctx, p, dir); err != nil {
			return err
		}
	}
	dest, err := app.Transcripts.Copy(p.Session, dir, job.options)
	if err != nil {
		return err
	}
	app.say("copied %s (%s)\n  from %s\n  to   %s", shell.Printable(p.Label()), p.ShortID, filepath.Dir(p.Path), dest)
	switch {
	case job.remove:
		if err := app.Agents.Remove(context.WithoutCancel(ctx), job.id); err != nil {
			return fmt.Errorf("copied, but cannot remove its background job: %w. Remove it with claudes gc %s, so claudes session resume --bg starts a new job in %s", err, job.id, dir)
		}
		app.say("removed its stopped background job %s: claudes session resume --bg %s starts a new one in %s%s",
			job.id, p.ShortID, dir, withOptions(job.options))
	case job.keep != "":
		app.say("kept its background job %s, since %s. Until it is removed, claude --bg --resume wakes it in %s", job.id, job.keep, job.dir)
	}
	remove := shell.RemoveCommand(transcripts.Files(p.Session))
	if rm {
		if err := app.Transcripts.RemoveOriginal(p.Session, dest); err != nil {
			return fmt.Errorf("copied, but %w. Remove it by hand with: %s", err, remove)
		}
		app.say("removed the original from %s", filepath.Dir(p.Path))
	} else {
		app.say("\nThe original stays, so it still resumes from %s. Remove it with:\n  %s", p.Cwd, remove)
	}
	if resume {
		p.Cwd = dir
		return app.resumePast(ctx, p, "", false)
	}
	app.say("\nResume it with:\n  %s", shell.ResumeCommand(p.SessionID, dir, true))
	if host {
		app.say("\nThis is the session running claudes. Exit it now: what it says from here on stays in the old transcript.")
	}
	return nil
}

// movingJob is what session mv does with a session's background job.
type movingJob struct {
	id, dir string
	// options are the job's saved options, which the move keeps for the job
	// claude --bg --resume starts once this one is gone.
	options []string
	remove  bool
	// keep says why a job that would wake in its old directory stays.
	keep string
}

// jobToMove looks up p's background job. A stopped one is removed, since
// claude --bg --resume wakes a job in its own directory, wherever it is run.
// A paused one stays, since claudes resume wakes it where it was paused.
func (app *App) jobToMove(ctx context.Context, p pastSession, dir string) (movingJob, error) {
	job := movingJob{options: p.JobOptions()}
	s, found, err := app.listedJob(ctx, p.SessionID)
	if err != nil || !found {
		return job, err
	}
	options, ok := app.Jobs.Options(s.JobID, s.SessionID)
	if ok {
		job.options = options
	}
	job.id, job.dir = s.JobID, cmp.Or(app.Jobs.Folder(s.JobID, s.SessionID), s.Cwd)
	if sameFolder(job.dir, dir) {
		return movingJob{options: job.options}, nil
	}
	parked, err := app.Park.Load()
	switch {
	case !ok:
		job.keep = "claudes cannot read the options Claude Code saved for it"
	case !isFinished(s):
		job.keep = fmt.Sprintf("it is not stopped (%s)", dash(s.State))
	case err != nil:
		job.keep = fmt.Sprintf("claudes cannot tell whether it is paused: %v", err)
	case slices.ContainsFunc(parked, func(e park.Entry) bool { return e.SessionID == s.SessionID }):
		job.keep = "it is paused, and claudes resume wakes it there"
	default:
		job.remove = true
	}
	return job, nil
}

// withOptions shows the options a new job gets, if any.
func withOptions(options []string) string {
	if len(options) == 0 {
		return ""
	}
	return ", with " + shell.Printable(strings.Join(options, " "))
}

// realFolder is directory as an absolute path with links resolved, as the
// working directory Claude Code records for a session started there.
func realFolder(folder string) (string, error) {
	abs, err := filepath.Abs(folder)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist. Create or clone it first", abs)
	}
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", abs)
	}
	return resolved, nil
}
