package shell

import (
	"strings"
	"testing"
)

func TestPOSIXResumeQuotesTheFolder(t *testing.T) {
	if got := posixResume("/a b/it's", "claude --bg --resume", "sid"); got != `cd '/a b/it'\''s' && claude --bg --resume sid` {
		t.Fatalf("command %q", got)
	}
	if got := posixResume("/plain/path", "claude --resume", "sid"); got != "cd /plain/path && claude --resume sid" {
		t.Fatalf("command %q", got)
	}
}

func TestPowerShellResumeQuotesEveryKindOfSingleQuote(t *testing.T) {
	got := powerShellResume("C:\\it's\u2019; echo x; \u2018", "claude --resume", "sid")
	want := "Set-Location -LiteralPath 'C:\\it''s\u2019\u2019; echo x; \u2018\u2018' -ErrorAction Stop; claude --resume sid"
	if got != want {
		t.Fatalf("command\n got: %s\nwant: %s", got, want)
	}
}

func TestPowerShellQuoteKeepsTheStopParsingTokenAnArgument(t *testing.T) {
	if got := powerShellResume("--%", "claude --resume", "--%"); got != "Set-Location -LiteralPath ('--%') -ErrorAction Stop; claude --resume ('--%')" {
		t.Fatalf("command %s", got)
	}
	if got := PowerShellQuote("--%x"); got != "'--%x'" {
		t.Fatalf("quoted %s", got)
	}
}

func TestPowerShellResumeQuotesAnOddSessionID(t *testing.T) {
	if got := powerShellResume("C:\\x", "claude --resume", "a;b"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; claude --resume 'a;b'" {
		t.Fatalf("command %s", got)
	}
	if got := powerShellResume("C:\\x", "claude --resume", "16cf5295-0000"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; claude --resume 16cf5295-0000" {
		t.Fatalf("a plain id was quoted: %s", got)
	}
}

func TestResumeCommandResumesInTheBackgroundOnlyForBackgroundSessions(t *testing.T) {
	if got := ResumeCommand("sid", "/x", false); got != resumeCommand("/x", "claude --bg --resume", "sid") {
		t.Fatalf("background command %q", got)
	}
	if got := ResumeCommand("sid", "/x", true); got != resumeCommand("/x", "claude --resume", "sid") {
		t.Fatalf("interactive command %q", got)
	}
}

func TestResumeCommandInSetsOrUnsetsTheConfigDir(t *testing.T) {
	if got := posixResume("/x", posixConfig("/p q")+"claude --resume", "sid"); got != "cd /x && CLAUDE_CONFIG_DIR='/p q' claude --resume sid" {
		t.Fatalf("POSIX command %q", got)
	}
	if got := posixResume("/x", posixConfig("")+"claude --resume", "sid"); got != "cd /x && env -u CLAUDE_CONFIG_DIR claude --resume sid" {
		t.Fatalf("POSIX command %q", got)
	}
	if got := powerShellResume("C:\\x", powerShellConfig("C:\\p")+"claude --resume", "sid"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; $env:CLAUDE_CONFIG_DIR = 'C:\\p'; claude --resume sid" {
		t.Fatalf("PowerShell command %q", got)
	}
	if got := powerShellResume("C:\\x", powerShellConfig("")+"claude --resume", "sid"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; $env:CLAUDE_CONFIG_DIR = $null; claude --resume sid" {
		t.Fatalf("PowerShell command %q", got)
	}
	if got, want := ResumeCommandIn("/p\x1b", "sid", "/x", false), resumeCommand("/x", configPrefix("/p")+"claude --bg --resume", "sid"); got != want {
		t.Fatalf("command %q, want %q", got, want)
	}
}

// A control character dropped from a value must not leave the quoting of
// the text before it: --\x01% prints as --%, PowerShell's stop-parsing token.
func TestCommandsQuoteTheTextTheyPrint(t *testing.T) {
	if got, want := ResumeCommand("s\x1bid", "--\x01%", true), resumeCommand("--%", "claude --resume", "sid"); got != want {
		t.Fatalf("resume %q, want %q", got, want)
	}
	if got, want := RemoveCommand([]string{"--\x01%", "/a\nb"}), removeCommand([]string{"--%", "/a b"}); got != want {
		t.Fatalf("remove %q, want %q", got, want)
	}
}

func TestRemoveCommands(t *testing.T) {
	paths := []string{"/a b/x.jsonl", "/a b/x"}
	if got := posixRemove(paths); got != "rm -r '/a b/x.jsonl' '/a b/x'" {
		t.Fatalf("posix %q", got)
	}
	if got := powerShellRemove(paths); got != "Remove-Item -Recurse -LiteralPath '/a b/x.jsonl', '/a b/x'" {
		t.Fatalf("powershell %q", got)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{"": "''", "plain/path-1": "plain/path-1", "it's": `'it'\''s'`, "a b": "'a b'"} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintableDropsControlCharacters(t *testing.T) {
	if got := Printable("name\nrm -rf ~\x1b[2J"); got != "name rm -rf ~[2J" {
		t.Fatalf("got %q", got)
	}
}

func TestPrintableDropsFormatCharactersButKeepsTheJoiner(t *testing.T) {
	in := "a\u202eb\u2066c\u200bd\u2028e\u200df\tg\rh"
	if got := Printable(in); got != "abcde\u200df g h" {
		t.Fatalf("got %q", got)
	}
	if got := PrintableText("x\ny\x1b[2J\tz"); got != "x\ny[2J\tz" {
		t.Fatalf("text %q", got)
	}
}

func TestQuoteQuotesALeadingTildeOrEquals(t *testing.T) {
	for in, want := range map[string]string{"~": "'~'", "~/x": "'~/x'", "=ls": "'=ls'", "a~b": "a~b", "a=b": "a=b"} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintableJoin(t *testing.T) {
	if got := PrintableJoin([]string{"api", "evil\x1b[2J", "db"}); got != "api, "+Printable("evil\x1b[2J")+", db" || strings.Contains(got, "\x1b") {
		t.Fatalf("got %q", got)
	}
	if got := PrintableJoin(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestInitCoversEachShell(t *testing.T) {
	for _, name := range []string{"bash", "zsh", "fish", "pwsh", "powershell"} {
		if script, ok := Init(name); !ok || !strings.Contains(script, "session where --one") {
			t.Errorf("%s: %v\n%s", name, ok, script)
		}
	}
	if _, ok := Init("tcsh"); ok {
		t.Error("tcsh has an integration")
	}
}
