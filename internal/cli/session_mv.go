package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/shell"
	"github.com/uvw/claudes/internal/transcripts"
)

func (app *App) sessionMvCommand() *cobra.Command {
	var resume, rm bool
	cmd := &cobra.Command{
		Use:     "mv TARGET FOLDER",
		Aliases: []string{"move"},
		Short:   "Copy a session to another folder, so it resumes from there",
		Long: `Copy a session's transcript into FOLDER's project folder, so claude --resume
finds it when started there. The original stays, so the session still resumes
from its old folder too; claudes prints the command that removes it. The old
folder's project memory and the folder recorded inside the transcript are left
alone. --rm deletes the original once the copy matches it byte for byte.

A running session is refused, since what it says after the copy stays in the
old transcript. The session running claudes is the exception: copy it, then
exit it and resume it from FOLDER. --resume resumes the copy in this terminal.`,
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
	cmd.Flags().BoolVar(&resume, "resume", false, "resume the session from FOLDER once it is copied")
	cmd.Flags().BoolVar(&rm, "rm", false, "delete the original once the copy matches it")
	app.addForce(cmd, "copy although Claude Code may name project folders differently now")
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
		if err := app.refuse(app.Transcripts.Dir, []string{"recent sessions live in folders claudes would not name that way"}); err != nil {
			return err
		}
	}
	if sameFolder(p.Cwd, dir) {
		return fmt.Errorf("%s already lives under %s", p.Label(), dir)
	}
	dest, err := app.Transcripts.Copy(p.Session, dir)
	if err != nil {
		return err
	}
	app.say("copied %s (%s)\n  from %s\n  to   %s", shell.Printable(p.Label()), p.ShortID, filepath.Dir(p.Path), dest)
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

// realFolder is folder as an absolute path with links resolved, as the
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
		return "", fmt.Errorf("%s is not a folder", abs)
	}
	return resolved, nil
}
