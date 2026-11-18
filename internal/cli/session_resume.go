package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/exit"
)

func (app *App) sessionResumeCommand() *cobra.Command {
	var bg bool
	cmd := &cobra.Command{
		Use:   "resume TARGET [PROMPT]",
		Short: "Resume a past session in its own directory, from any directory",
		Long: `Resume a past session in this terminal, in the directory it ran in, with an
optional prompt. --bg resumes it in the background instead. A session that is
still running is refused: resuming it would start a copy. When session mv
removed the session's background job, --bg starts a new job with the old
job's options, such as --model.`,
		Args:              cobra.RangeArgs(1, 2),
		ValidArgsFunction: app.completePast(true),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := ""
			if len(args) == 2 {
				prompt = args[1]
			}
			if err := app.Agents.CheckPrompt(prompt); err != nil {
				return err
			}
			p, err := app.onePast(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if p.Running {
				return exit.Refusal(fmt.Errorf("%s is still running, and resuming it would start a copy. Use claudes attach, or stop it first", p.Label()))
			}
			if !app.runningKnown {
				return exit.Refusal(fmt.Errorf("cannot tell whether %s is running, and resuming a running session starts a copy: try again", p.Label()))
			}
			return app.resumePast(cmd.Context(), p, prompt, bg)
		},
	}
	cmd.Flags().BoolVarP(&bg, "bg", "b", false, "resume in the background, as claude --bg does")
	app.addForce(cmd, "with --bg, resume although claude agents lists something claudes does not recognize")
	return cmd
}
