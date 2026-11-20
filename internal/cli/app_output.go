package cli

import (
	"cmp"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/render"
	"github.com/uvw/claudes/internal/shell"
)

// addOutput registers -o on cmd as out, defaulting to format, or to text when
// format is "". config.Load has already checked a format from config.yaml.
func (app *App) addOutput(cmd *cobra.Command, out *render.Format, format string) {
	*out = render.Format(cmp.Or(format, string(render.Text)))
	cmd.Flags().VarP(out, "output", "o", "print as text, json, or yaml")
	cmd.RegisterFlagCompletionFunc("output", cobra.FixedCompletions(render.Formats, cobra.ShellCompDirectiveNoFileComp))
}

// tableFit says how writeTable fits a table to the line.
type tableFit struct {
	// margin is how many columns go before each line.
	margin int
	// fitLast cuts the last column to what is left of the terminal's width,
	// or to lastCap off a terminal when lastCap is not 0.
	fitLast bool
	lastCap int
}

// writeTable writes rows, the first being the header, as render.Table does,
// after render.Fit fits them to Out's terminal and output.dirs. While a line is
// wider than the terminal, it cuts the last column with fitLast, then each
// directory, then each name, then each branch.
func (app *App) writeTable(w io.Writer, rows [][]string, fit tableFit) error {
	var cols []render.Column
	if len(rows) > 0 {
		cols = render.Columns(rows[0], fit.fitLast, fit.lastCap)
	}
	render.Fit(rows, render.Layout{Width: app.width(), Margin: fit.margin, Dirs: app.dirs(), Columns: cols}, app.colors.Faint)
	return render.Table(w, rows)
}

// dirIn is path as a line of text shows it, such as a heading, shortened as a
// table shortens a directory. used is how many columns the rest of the line
// takes.
func (app *App) dirIn(path string, used int) string {
	return render.ShortDir(render.HomeShort(path), render.DirRoom(app.width(), used), app.dirs())
}

// dirs is output.dirs, or its default.
func (app *App) dirs() string { return cmp.Or(app.Config.Output.Dirs, render.DirsMiddle) }

// width is how many columns the terminal Out writes to has, or 0 when Out is
// not a terminal.
func (app *App) width() int {
	if app.Width == nil {
		return 0
	}
	return max(app.Width(), 0)
}

// dirRow is one row of the table where and session where print.
type dirRow struct{ id, dir, name string }

// printDirs prints one directory bare, so cd "$(claudes where NAME)" works, or
// several as a table of id, directory, and name, so sessions that share a name
// stay apart.
func (app *App) printDirs(found []dirRow) error {
	if len(found) == 1 {
		_, err := fmt.Fprintln(app.Out, shell.Printable(found[0].dir))
		return err
	}
	table := [][]string{{"ID", "DIR", "NAME"}}
	for _, f := range found {
		table = append(table, []string{f.id, render.DirCell(f.dir), render.NameCell(f.name)})
	}
	return app.writeTable(app.Out, table, tableFit{})
}

// say prints to stdout, unless the command prints a report there instead.
func (app *App) say(format string, args ...any) {
	if app.rep == nil {
		fmt.Fprintf(app.Out, format+"\n", args...)
	}
}

func (app *App) warn(format string, args ...any) {
	fmt.Fprintf(app.Err, format+"\n", args...)
	app.record(func(r *report) {
		if !r.printed {
			r.Warnings = append(r.Warnings, strings.TrimPrefix(fmt.Sprintf(format, args...), "claudes: "))
		}
	})
}

// errorLine prints a line to stderr, in red when stderr shows colors.
func (app *App) errorLine(format string, args ...any) {
	fmt.Fprintln(app.Err, app.errColors.Red(fmt.Sprintf(format, args...)))
}

// logOff is above every level slog has, so nothing is logged.
const logOff = slog.Level(100)

// log is the diagnostic log on stderr: each claude call claudes makes, with
// its time and result, and the steps a command takes. It is off unless
// CLAUDES_LOG names a level or -v turns on debug. What a command reports to
// the user goes through say and warn, never here.
func (app *App) log() *slog.Logger {
	if app.logger == nil {
		app.logLevel = new(slog.LevelVar)
		app.logLevel.Set(logOff)
		app.logger = slog.New(slog.NewTextHandler(errWriter{app}, &slog.HandlerOptions{
			Level: app.logLevel,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// A CLI's lines are read as they appear, so the time is noise.
				if len(groups) == 0 && a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				return a
			},
		}))
	}
	return app.logger
}

// setLogLevel turns the log on at a level named as CLAUDES_LOG takes it:
// debug, info, warn, or error.
func (app *App) setLogLevel(name string) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(name))); err != nil {
		return fmt.Errorf("CLAUDES_LOG must be debug, info, warn, or error, not %q", name)
	}
	app.log()
	app.logLevel.Set(level)
	return nil
}

// errWriter writes to the App's stderr as it is when the line is logged, so a
// test that swaps Err sees the log too.
type errWriter struct{ app *App }

func (w errWriter) Write(p []byte) (int, error) { return w.app.Err.Write(p) }
