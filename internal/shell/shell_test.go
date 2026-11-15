package shell

import "testing"

func TestPOSIXResumeQuotesTheFolder(t *testing.T) {
	if got := posixResume("/a b/it's", "--bg --resume", "sid"); got != `cd '/a b/it'\''s' && claude --bg --resume sid` {
		t.Fatalf("command %q", got)
	}
	if got := posixResume("/plain/path", "--resume", "sid"); got != "cd /plain/path && claude --resume sid" {
		t.Fatalf("command %q", got)
	}
}

func TestPowerShellResumeQuotesEveryKindOfSingleQuote(t *testing.T) {
	got := powerShellResume("C:\\it's\u2019; echo x; \u2018", "--resume", "sid")
	want := "Set-Location -LiteralPath 'C:\\it''s\u2019\u2019; echo x; \u2018\u2018' -ErrorAction Stop; claude --resume sid"
	if got != want {
		t.Fatalf("command\n got: %s\nwant: %s", got, want)
	}
}

func TestPowerShellResumeQuotesAnOddSessionID(t *testing.T) {
	if got := powerShellResume("C:\\x", "--resume", "a;b"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; claude --resume 'a;b'" {
		t.Fatalf("command %s", got)
	}
	if got := powerShellResume("C:\\x", "--resume", "16cf5295-0000"); got != "Set-Location -LiteralPath 'C:\\x' -ErrorAction Stop; claude --resume 16cf5295-0000" {
		t.Fatalf("a plain id was quoted: %s", got)
	}
}

func TestResumeCommandResumesInTheBackgroundOnlyForBackgroundSessions(t *testing.T) {
	if got := ResumeCommand("sid", "/x", false); got != resumeCommand("/x", "--bg --resume", "sid") {
		t.Fatalf("background command %q", got)
	}
	if got := ResumeCommand("sid", "/x", true); got != resumeCommand("/x", "--resume", "sid") {
		t.Fatalf("interactive command %q", got)
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
