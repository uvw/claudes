package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/skills"
)

var (
	fenced = regexp.MustCompile("(?s)```[a-z]*\n(.*?)```")
	inline = regexp.MustCompile("`([^`\n]+)`")
	// A shell joins commands with these; each part is checked on its own.
	shellJoin = regexp.MustCompile(`\|\||&&|[|;]|\$\(|\)`)
)

// skillCommands is every `claudes ...` invocation the skill shows in code,
// inline or in a block, without the prose around it.
func skillCommands(doc string) []string {
	var code []string
	for _, m := range fenced.FindAllStringSubmatch(doc, -1) {
		code = append(code, strings.Split(m[1], "\n")...)
	}
	for _, m := range inline.FindAllStringSubmatch(fenced.ReplaceAllString(doc, ""), -1) {
		code = append(code, m[1])
	}
	var out []string
	for _, line := range code {
		line, _, _ = strings.Cut(line, "#")
		for _, part := range shellJoin.Split(line, -1) {
			if fields := strings.Fields(part); len(fields) > 0 && fields[0] == "claudes" {
				out = append(out, strings.Join(fields, " "))
			}
		}
	}
	return out
}

// checkInvocation walks the command tree along words, then checks every flag
// against the command it reached. It returns what does not exist, or "".
func checkInvocation(root *cobra.Command, words []string) string {
	cmd, rest := root, words
	for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") && !placeholder(rest[0]) {
		next, _, err := cmd.Find(rest[:1])
		if err != nil || next == cmd {
			break
		}
		cmd, rest = next, rest[1:]
	}
	if cmd == root && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") && !placeholder(rest[0]) {
		return "no command " + rest[0]
	}
	for _, w := range rest {
		if !strings.HasPrefix(w, "-") || w == "--" {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimLeft(w, "-"), "=")
		// cobra adds --help to every command, and --version to the root, only
		// when it runs one.
		if name == "help" || name == "h" || name == "version" && cmd == root {
			continue
		}
		var found bool
		if strings.HasPrefix(w, "--") {
			found = cmd.Flags().Lookup(name) != nil || cmd.InheritedFlags().Lookup(name) != nil
		} else {
			found = cmd.Flags().ShorthandLookup(name[:1]) != nil || cmd.InheritedFlags().ShorthandLookup(name[:1]) != nil
		}
		if !found {
			return cmd.CommandPath() + " has no flag " + w
		}
	}
	return ""
}

func TestSkillNamesOnlyCommandsAndFlagsThatExist(t *testing.T) {
	h := newHarness(t)
	root := h.app.rootCommand()
	commands := skillCommands(string(skills.Claudes))
	if len(commands) < 30 {
		t.Fatalf("found only %d claudes commands in SKILL.md; the parsing missed most", len(commands))
	}
	for _, c := range commands {
		if problem := checkInvocation(root, strings.Fields(c)[1:]); problem != "" {
			t.Errorf("SKILL.md shows %q, but %s", c, problem)
		}
	}
}

func TestCheckInvocationCatchesWhatDoesNotExist(t *testing.T) {
	root := newHarness(t).app.rootCommand()
	for words, want := range map[string]string{
		"restart --ping NAME":       "",
		"session ls --since 7d":     "",
		"ls -a plan":                "",
		"restart --pong":            "claudes restart has no flag --pong",
		"sesion ls":                 "no command sesion",
		"gc --teams --dry-run -o x": "",
		"<command> --help":          "",
		"--version":                 "",
		"logs --version":            "claudes logs has no flag --version",
	} {
		if got := checkInvocation(root, strings.Fields(words)); got != want {
			t.Errorf("%q: got %q, want %q", words, got, want)
		}
	}
}

// placeholder reports whether w stands for something the reader fills in, such
// as <command>, NAME, or ID.
func placeholder(w string) bool {
	return strings.HasPrefix(w, "<") || w == strings.ToUpper(w) && strings.ToLower(w) != w
}
