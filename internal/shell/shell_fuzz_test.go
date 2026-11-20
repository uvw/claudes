package shell

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// call is one command a shell ran, as the PowerShell stand-ins and
// testdata/printargs print it: its name, then each argument in base64, so
// any text survives.
func call(name string, args []string) string {
	out := []string{name}
	for _, a := range args {
		out = append(out, base64.StdEncoding.EncodeToString([]byte(a)))
	}
	return strings.Join(out, " ")
}

// decode is calls[i] with its arguments decoded, or nil when there is none.
func decode(calls []string, i int) []string {
	if i >= len(calls) {
		return nil
	}
	words := strings.Split(calls[i], " ")
	for j, w := range words[1:] {
		arg, _ := base64.StdEncoding.DecodeString(w)
		words[j+1] = string(arg)
	}
	return words
}

// hostile are names and paths that a careless quoter lets a shell read as
// more than one word, or as code.
var hostile = []string{
	"", " ", "plain/path-1", "a b", "it's", "'", "''", `a'b'c`, `"`, `\`, `a\`, `\'`,
	"$HOME", "${HOME}", "$(touch pwned)", "`touch pwned`", "~", "~/x", "~root", "=ls", "a=b",
	"*", "?", "[a]", "{a,b}", "!", "!!", "#x", "a#b", ";", "&", "&&", "|", "<x", ">x", "(", ")",
	"-n", "-e", "--", "--%", "-LiteralPath", "@args", "$args", "%PATH%", "^", "1234e567", "0x10", "1kb",
	"\t", "\n", "\r\n", "a\nrm -rf ~", "\x1b[2J", "\x7f", "\u202e", "\u2028", "\u200d", "\ufeff",
	"‘", "’", "‚", "‛", "C:\\it's\u2019; echo x; \u2018", `C:\Program Files\x`, "名前", "🙂",
}

// addSeeds adds the hostile words. A fuzz input is one or more words split
// by NULs, which no argument can hold, so one shell checks many at once.
func addSeeds(f *testing.F) {
	f.Add(strings.Join(hostile, "\x00"))
	for _, s := range []string{"", "--%", "~\x00it's", "C:\\it's\u2019; echo x; \u2018\x00--\x00-LiteralPath\x001234e567",
		"a\nrm -rf ~\x00\x1b[2J\u202e\x00$(touch pwned)\x00`touch pwned`", "\xff\xfe\x00\xc3\x00\xed\xa0\x80"} {
		f.Add(s)
	}
}

// FuzzPOSIXCommands checks that every word claudes quotes for a POSIX shell
// reaches a real one unchanged: a word alone, and the cd, claude, and rm
// commands, as they are built and as ResumeCommand and RemoveCommand print
// them.
func FuzzPOSIXCommands(f *testing.F) {
	shells := posixShells()
	if len(shells) == 0 {
		f.Skip("no POSIX shell")
	}
	addSeeds(f)
	empty := f.TempDir()
	f.Fuzz(func(t *testing.T, input string) {
		words := strings.Split(input, "\x00")
		var script strings.Builder
		script.WriteString(`args() { printf '%s\0' args "$#" "$@"; }
cd() { printf '%s\0' cd "$#" "$@"; }
claude() { printf '%s\0' claude "$#" "$@"; }
rm() { printf '%s\0' rm "$#" "$@"; }
args ` + joinQuoted(words, Quote, " ") + "\n")
		want := [][]string{append([]string{"args"}, words...)}
		for i, w := range words {
			id := words[(i+1)%len(words)]
			script.WriteString(posixResume(w, "--bg --resume", id) + "\n" + ResumeCommand(id, w, true) + "\n")
			want = append(want, []string{"cd", w}, []string{"claude", "--bg", "--resume", id},
				[]string{"cd", Printable(w)}, []string{"claude", "--resume", Printable(id)})
		}
		script.WriteString(posixRemove(words) + "\n" + RemoveCommand(words) + "\n")
		want = append(want, append([]string{"rm", "-r"}, words...), []string{"rm", "-r"})
		for _, w := range words {
			want[len(want)-1] = append(want[len(want)-1], Printable(w))
		}
		for _, sh := range shells {
			cmd := exec.Command(sh[0], append(sh[1:], script.String())...)
			// An empty PATH and directory keep a quoting bug from running
			// anything real.
			cmd.Dir, cmd.Env = empty, []string{"PATH=" + empty, "HOME=" + empty, "LC_ALL=C.UTF-8"}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s: %v: %s\nscript:\n%s", sh[0], err, stderr.String(), script.String())
			}
			if got := posixCalls(t, out); !slices.EqualFunc(got, want, slices.Equal) {
				t.Fatalf("%s read\n%q\nwant\n%q\nscript:\n%s", sh[0], got, want, script.String())
			}
		}
	})
}

// posixCalls splits what the stand-ins print: each call's name, its count of
// arguments, and the arguments, all ended by a NUL.
func posixCalls(t *testing.T, out []byte) [][]string {
	t.Helper()
	words := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	var calls [][]string
	for len(words) >= 2 {
		n, err := strconv.Atoi(words[1])
		if err != nil || len(words) < 2+n {
			t.Fatalf("output %q", out)
		}
		calls = append(calls, append([]string{words[0]}, words[2:2+n]...))
		words = words[2+n:]
	}
	if len(words) != 0 {
		t.Fatalf("output %q", out)
	}
	return calls
}

// FuzzPowerShellCommands checks the same for PowerShell: each word quoted
// both ways, and the Set-Location, claude, and Remove-Item commands claudes
// prints on Windows. It runs wherever pwsh is installed.
func FuzzPowerShellCommands(f *testing.F) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		f.Skip("pwsh is not installed")
	}
	addSeeds(f)
	ps := startPowerShell(f, pwsh)
	f.Fuzz(func(t *testing.T, input string) {
		// PowerShell reads its script as text, so bytes that are not UTF-8
		// cannot reach it.
		if !utf8.ValidString(input) {
			t.Skip()
		}
		words := strings.Split(input, "\x00")
		// PowerShell passes the arguments after a --% to a program as they
		// are written, however the --% was quoted, so no quoting gets --%
		// itself to claude. Set-Location and Remove-Item still take it.
		ids := slices.DeleteFunc(slices.Clone(words), func(w string) bool { return Printable(w) == "--%" || w == "--%" })
		if len(ids) == 0 {
			ids = []string{"x"}
		}
		var script strings.Builder
		var twice []string
		script.WriteString("claude")
		for _, w := range ids {
			script.WriteString(" " + PowerShellQuote(w) + " " + powerShellArg(w))
			twice = append(twice, w, w)
		}
		script.WriteString("\n")
		want := []string{call("claude", twice)}
		// Each claude starts a process, which takes tens of milliseconds, so
		// the resume command runs for the first few words only. The other
		// lines take every word. It is built as ResumeCommand builds it on
		// Windows.
		for i, w := range words[:min(len(words), 4)] {
			id := ids[i%len(ids)]
			flags := [][]string{{"--resume"}, {"--bg", "--resume"}}[i%2]
			script.WriteString(powerShellResume(Printable(w), strings.Join(flags, " "), Printable(id)) + "\n")
			want = append(want, call("Set-Location", []string{Printable(w)}), call("claude", append(flags, Printable(id))))
		}
		script.WriteString(powerShellRemove(words) + "\n")
		want = append(want, call("Remove-Item", append([]string{"True"}, words...)))
		got := ps.run(t, script.String())
		for i := range max(len(got), len(want)) {
			if i >= len(got) || i >= len(want) || got[i] != want[i] {
				t.Fatalf("pwsh read call %d as\n%q\nwant\n%q\nscript:\n%s", i, decode(got, i), decode(want, i), script.String())
			}
		}
	})
}

// powerShellServer runs one script per line of its input, so an input does
// not pay for starting pwsh. Set-Location and Remove-Item are stand-ins that
// bind the parameters the real ones do, and claude is testdata/printargs, a
// real program, since PowerShell passes arguments to a program by rules of
// its own.
const powerShellServer = `$ErrorActionPreference = 'Stop'
function Out-Call([string]$name, [string[]]$values) {
  (@($name) + @($values | ForEach-Object { [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($_)) })) -join ' '
}
function Set-Location { [CmdletBinding()] param([string]$LiteralPath) Out-Call 'Set-Location' @($LiteralPath) }
function Remove-Item { [CmdletBinding()] param([switch]$Recurse, [string[]]$LiteralPath) Out-Call 'Remove-Item' (@("$Recurse") + $LiteralPath) }
Set-Alias claude $env:CLAUDES_TEST_CLAUDE
$env:PATH = ''
$tab = [string][char]9
while ($null -ne ($line = [Console]::In.ReadLine())) {
  try {
    $code = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($line))
    $calls = @(& ([scriptblock]::Create($code)))
    [Console]::Out.WriteLine('ok' + $tab + ($calls -join $tab))
  } catch {
    [Console]::Out.WriteLine('error' + $tab + ($_ | Out-String).Replace([string][char]10, ' '))
  }
  [Console]::Out.Flush()
}
`

type powerShell struct {
	in  io.Writer
	out *bufio.Reader
}

func startPowerShell(f *testing.F, pwsh string) *powerShell {
	dir := f.TempDir()
	server := filepath.Join(dir, "server.ps1")
	if err := os.WriteFile(server, []byte(powerShellServer), 0o600); err != nil {
		f.Fatal(err)
	}
	// Built on its own rather than run as the test binary, which starts in
	// about a second under the race detector.
	claude := filepath.Join(dir, "claude.exe")
	if out, err := exec.Command("go", "build", "-o", claude, "./testdata/printargs").CombinedOutput(); err != nil {
		f.Fatalf("building printargs: %v\n%s", err, out)
	}
	cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", server)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CLAUDES_TEST_CLAUDE="+claude)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		f.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		f.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		f.Fatal(err)
	}
	f.Cleanup(func() {
		in.Close()
		_ = cmd.Wait()
	})
	return &powerShell{in: in, out: bufio.NewReader(out)}
}

// run runs script and returns the calls it made.
func (p *powerShell) run(t *testing.T, script string) []string {
	t.Helper()
	if _, err := fmt.Fprintln(p.in, base64.StdEncoding.EncodeToString([]byte(script))); err != nil {
		t.Fatal(err)
	}
	read := make(chan error, 1)
	var line string
	go func() {
		var err error
		line, err = p.out.ReadString('\n')
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil {
			t.Fatalf("pwsh: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("pwsh did not finish\nscript:\n%s", script)
	}
	status, calls, _ := strings.Cut(strings.TrimRight(line, "\r\n"), "\t")
	if status != "ok" {
		t.Fatalf("pwsh: %s\nscript:\n%s", calls, script)
	}
	return strings.Split(calls, "\t")
}
