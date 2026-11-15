// Package shell writes the commands claudes prints for a person to run, in
// the shell each platform opens by default, and makes text safe to print.
package shell

import (
	"fmt"
	"strings"
	"unicode"
)

// ResumeCommand is the command that resumes a session by hand from its
// folder: claude --resume for an interactive session, claude --bg --resume
// for a background one.
func ResumeCommand(sessionID, cwd string, interactive bool) string {
	flags := "--bg --resume"
	if interactive {
		flags = "--resume"
	}
	return Printable(resumeCommand(cwd, flags, sessionID))
}

// RemoveCommand is the command that deletes paths and everything in them.
func RemoveCommand(paths []string) string { return Printable(removeCommand(paths)) }

func posixResume(cwd, flags, sessionID string) string {
	return fmt.Sprintf("cd %s && claude %s %s", Quote(cwd), flags, Quote(sessionID))
}

func posixRemove(paths []string) string {
	return "rm -r " + joinQuoted(paths, Quote, " ")
}

func powerShellResume(cwd, flags, sessionID string) string {
	id := powerShellArg(sessionID)
	return fmt.Sprintf("Set-Location -LiteralPath %s -ErrorAction Stop; claude %s %s", PowerShellQuote(cwd), flags, id)
}

func powerShellRemove(paths []string) string {
	return "Remove-Item -Recurse -LiteralPath " + joinQuoted(paths, PowerShellQuote, ", ")
}

// powerShellArg quotes s for PowerShell unless it is letters, digits, and
// dashes only.
func powerShellArg(s string) string {
	if s != "" && !strings.ContainsFunc(s, func(r rune) bool { return r != '-' && !isASCIIAlnum(r) }) {
		return s
	}
	return PowerShellQuote(s)
}

func joinQuoted(values []string, quote func(string) string, sep string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return strings.Join(quoted, sep)
}

// Quote quotes s for a POSIX shell when it needs quoting.
func Quote(s string) string {
	if s == "" {
		return "''"
	}
	// A leading ~ or = expands in some shells, so it is quoted too.
	plain := !strings.ContainsFunc(s, func(r rune) bool { return !isASCIIAlnum(r) && !strings.ContainsRune("@%+=:,./_-~", r) })
	if plain && s[0] != '~' && s[0] != '=' {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PowerShellQuote quotes s as a PowerShell single-quoted string. PowerShell
// reads the curly single quotes as quotes too, so each of them is doubled,
// like the straight one.
func PowerShellQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch r {
		case '\'', '‘', '’', '‚', '‛':
			b.WriteRune(r)
		}
		b.WriteRune(r)
	}
	b.WriteByte('\'')
	return b.String()
}

// Printable makes s safe to print on one line: tabs and line breaks become
// spaces, and other control and format characters are dropped, so a session
// name cannot start a line of its own, send a terminal escape sequence, or
// reverse the text after it.
func Printable(s string) string { return printable(s, false) }

// PrintableText is Printable that keeps line breaks and tabs, for text that
// spans lines, such as an error claude printed.
func PrintableText(s string) string { return printable(s, true) }

func printable(s string, keepLines bool) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			if keepLines {
				return r
			}
			return ' '
		case r == '\r':
			return ' '
		case unicode.IsControl(r), unicode.In(r, unicode.Zl, unicode.Zp):
			return -1
		case unicode.Is(unicode.Cf, r) && r != '\u200d':
			// Format characters include the bidi overrides. The zero-width
			// joiner stays, since emoji sequences use it.
			return -1
		}
		return r
	}, s)
}

// Arg is s as one argument for the shell this platform's printed commands
// use, quoted only when it needs to be.
func Arg(s string) string { return arg(s) }

func isASCIIAlnum(r rune) bool {
	return r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r))
}
