// Package style shapes what claudes prints: its colors, which reach only a
// terminal; its text tables, fitted to the terminal's width; the directories
// those tables shorten; and sizes as du prints them.
package style

import (
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Palette styles text for one output stream. The zero Palette leaves text
// plain.
type Palette struct{ on bool }

// On is a Palette that colors whatever the stream is.
var On = Palette{on: true}

// Detect decides whether text written to w gets colors. env is the
// environment, as os.Environ returns it. It follows the clicolors convention
// (https://bixense.com/clicolors/), where an empty variable counts as unset.
// In that order, NO_COLOR turns colors off, CLICOLOR_FORCE or FORCE_COLOR,
// unless 0, turns them on even when w is not a terminal, and CLICOLOR=0 turns
// them off. Otherwise w gets colors when it is a terminal that shows them.
func Detect(w io.Writer, env []string) Palette {
	if lookup(env, "NO_COLOR") != "" {
		return Palette{}
	}
	if !forced(env, "CLICOLOR_FORCE") && !forced(env, "FORCE_COLOR") {
		if lookup(env, "CLICOLOR") == "0" || !isTerminal(w) || !shows(w, env) {
			return Palette{}
		}
	}
	if f, ok := w.(*os.File); ok {
		// The console in Windows 10 and later shows colors only once asked to.
		lipgloss.EnableLegacyWindowsANSI(f)
	}
	return On
}

// forced reports whether env sets key to force colors: to anything but 0.
func forced(env []string, key string) bool {
	v := lookup(env, key)
	return v != "" && v != "0"
}

// isTerminal reports whether w is a terminal. Tests replace it.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}

// shows reports whether the terminal w shows colors, such as one with
// TERM=dumb does not. Detect settles CLICOLOR_FORCE before it asks, and
// colorprofile's other way to force colors on a pipe, TTY_FORCE, is left out.
func shows(w io.Writer, env []string) bool {
	kept := []string{"TTY_FORCE=1"}
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k != "CLICOLOR_FORCE" && k != "TTY_FORCE" {
			kept = append(kept, kv)
		}
	}
	return colorprofile.Detect(w, kept) >= colorprofile.ANSI
}

// lookup is the last value env gives key, or "".
func lookup(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			value = v
		}
	}
	return value
}

// Colors reports whether p styles text.
func (p Palette) Colors() bool { return p.on }

var (
	red     = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Red)
	green   = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Green)
	yellow  = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Yellow)
	blue    = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.BrightBlue)
	magenta = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Magenta)
	cyan    = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Foreground(lipgloss.Cyan)
	faint   = lipgloss.NewStyle().TabWidth(lipgloss.NoTabConversion).Faint(true)
)

func (p Palette) render(s lipgloss.Style, text string) string {
	if !p.on || text == "" {
		return text
	}
	// Each line on its own, since Render pads the lines of a block to one
	// width and turns tabs into spaces.
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = s.Render(l)
		}
	}
	return strings.Join(lines, "\n")
}

// Red is for failures and errors.
func (p Palette) Red(text string) string { return p.render(red, text) }

// Green is for what worked or is at work.
func (p Palette) Green(text string) string { return p.render(green, text) }

// Yellow is for warnings and what is out of date.
func (p Palette) Yellow(text string) string { return p.render(yellow, text) }

// Blue marks a choice the user made, such as a pin.
func (p Palette) Blue(text string) string { return p.render(blue, text) }

// Magenta is for a session that waits on the user.
func (p Palette) Magenta(text string) string { return p.render(magenta, text) }

// Cyan is for a session at a shell prompt.
func (p Palette) Cyan(text string) string { return p.render(cyan, text) }

// Faint is for detail the eye can skip, such as a directory.
func (p Palette) Faint(text string) string { return p.render(faint, text) }

// Width is how many columns text takes in a terminal, without its colors.
func Width(text string) int { return lipgloss.Width(text) }

// Pad styles text with paint and pads it with spaces to width columns, so a
// colored word lines up as a plain one would.
func Pad(text string, width int, paint func(string) string) string {
	return paint(text) + strings.Repeat(" ", max(0, width-Width(text)))
}
