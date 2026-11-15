package cli

import (
	"fmt"
	"log/slog"
	"strings"
)

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
