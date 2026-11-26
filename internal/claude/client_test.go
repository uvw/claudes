package claude

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/shell"
)

// helperEnv makes the test binary act as a stand-in for claude: "parent"
// starts a child that inherits stdout, prints its PID, and exits at once, and
// "child" holds stdout open for ten seconds. "fail-stdout" and "fail-silent"
// exit non-zero, with a message on stdout and with nothing at all.
const helperEnv = "CLAUDES_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperEnv+"=child")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Println("started", child.Process.Pid)
		os.Exit(0)
	case "child":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "exec":
		// Exec stands in for claudes handing the console to claude agents.
		os.Setenv(helperEnv, "exec-child")
		fmt.Fprintln(os.Stderr, Exec(ConfigDir{}, os.Args[0]))
		os.Exit(2)
	case "exec-child":
		// The child starts a process of its own, as claude agents starts the
		// background service, then waits.
		grand := exec.Command(os.Args[0])
		grand.Env = append(os.Environ(), helperEnv+"=child")
		if err := grand.Start(); err != nil {
			os.Exit(2)
		}
		fmt.Println(os.Getpid(), grand.Process.Pid)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "env":
		dir, set := os.LookupEnv("CLAUDE_CONFIG_DIR")
		fmt.Printf("CLAUDECODE=%s JOB=%s PATH-set=%v CONFIG=%s CONFIG-set=%v", os.Getenv("CLAUDECODE"), os.Getenv("CLAUDE_JOB_DIR"), os.Getenv("PATH") != "", dir, set)
		os.Exit(0)
	case "fail-stdout":
		fmt.Println("  Not logged in  ")
		os.Exit(2)
	case "fail-silent":
		os.Exit(3)
	}
	os.Exit(m.Run())
}

type fakeRunner struct {
	calls   [][]string
	dirs    []string
	configs []ConfigDir
	stdout  string
	stderr  string
	err     error
	// checkErr is what Check returns, as for a binary that cannot take an
	// argument.
	checkErr error
}

func (f *fakeRunner) Check(...string) error { return f.checkErr }

func (f *fakeRunner) Run(_ context.Context, dir string, config ConfigDir, args ...string) (string, string, error) {
	f.calls = append(f.calls, args)
	f.dirs = append(f.dirs, dir)
	f.configs = append(f.configs, config)
	return f.stdout, f.stderr, f.err
}

func TestSessionsParsesTheAgentsJSON(t *testing.T) {
	r := &fakeRunner{stdout: `[
	  {"id":"242e8b07","sessionId":"242e8b07-a8ec-4656-900e-0f340368feb7","name":"db","cwd":"/x","kind":"background","state":"working","status":"busy","pid":92942,"startedAt":1794589202645},
	  {"sessionId":"16cf5295-0000-0000-0000-000000000000","cwd":"/y","kind":"interactive","status":"idle","pid":80359,"startedAt":1}
	]`}
	sessions, err := (&Client{Runner: r}).Sessions(t.Context(), ConfigDir{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.calls[0], []string{"agents", "--json", "--all"}) {
		t.Fatalf("args %q", r.calls[0])
	}
	db, tty := sessions[0], sessions[1]
	if !db.Running() || !db.Busy() || db.Interactive() || db.ID() != "242e8b07" {
		t.Fatalf("db %+v", db)
	}
	if !tty.Interactive() || tty.JobID != "" || tty.Label() != "16cf5295" {
		t.Fatalf("tty %+v", tty)
	}
	if db.Mode() != "bg" || tty.Mode() != "tty" {
		t.Errorf("modes %q and %q, want bg and tty", db.Mode(), tty.Mode())
	}
}

func TestSessionsLeavesOutARecordWithoutASessionID(t *testing.T) {
	r := &fakeRunner{stdout: `[{"name":"x","kind":"interactive"},{"sessionId":"s-1","kind":"background"}]`}
	sessions, err := (&Client{Runner: r}).Sessions(t.Context(), ConfigDir{}, false)
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "s-1" {
		t.Fatalf("sessions %+v, err %v", sessions, err)
	}
}

func TestMidTurnCoversEveryStatusThatHasWorkInFlight(t *testing.T) {
	for status, want := range map[string]bool{"busy": true, "waiting": true, "shell": true, "idle": false, "": false} {
		if got := (Session{Status: status}).MidTurn(); got != want {
			t.Errorf("%q: MidTurn %v", status, got)
		}
	}
}

func TestAWorkingSessionIsBusyBeforeItsStatusCatchesUp(t *testing.T) {
	s := Session{State: "working", Status: "idle"}
	if !s.Busy() || !s.MidTurn() || s.Activity() != "busy" {
		t.Fatalf("busy %v, mid-turn %v, activity %q", s.Busy(), s.MidTurn(), s.Activity())
	}
	done := Session{State: "done", Status: "idle"}
	if done.Busy() || done.MidTurn() || done.Activity() != "idle" {
		t.Fatalf("a finished turn: busy %v, mid-turn %v", done.Busy(), done.MidTurn())
	}
}

// blockingRunner never returns until its context ends.
type blockingRunner struct{}

func (blockingRunner) Check(...string) error { return nil }

func (blockingRunner) Run(ctx context.Context, _ string, _ ConfigDir, _ ...string) (string, string, error) {
	<-ctx.Done()
	return "", "", errors.New("signal: killed")
}

func TestACallThatHangsTimesOut(t *testing.T) {
	c := &Client{Runner: blockingRunner{}, ActionTimeout: 20 * time.Millisecond}
	err := c.Stop(t.Context(), ConfigDir{}, "242e8b07")
	if err == nil || !strings.Contains(err.Error(), "claude stop 242e8b07 timed out after 20ms") {
		t.Fatalf("err %v", err)
	}
}

func TestExecRunnerReturnsWhenAGrandchildHoldsStdout(t *testing.T) {
	t.Setenv(helperEnv, "parent")
	done := make(chan error, 1)
	var out string
	go func() {
		var err error
		out, _, err = execRunner{Binary: os.Args[0], WaitDelay: 200 * time.Millisecond}.Run(t.Context(), "", ConfigDir{})
		done <- err
	}()
	select {
	case err := <-done:
		var pid int
		if _, scanErr := fmt.Sscanf(out, "started %d", &pid); err != nil || scanErr != nil {
			t.Fatalf("out %q, err %v", out, err)
		}
		if p, err := os.FindProcess(pid); err == nil {
			p.Kill()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run still blocked after 5s")
	}
}

func TestCallsToClaudeLeaveOutTheSessionsOwnVariables(t *testing.T) {
	t.Setenv(helperEnv, "env")
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_JOB_DIR", "/jobs/ff78d735")
	out, _, err := execRunner{Binary: os.Args[0]}.Run(t.Context(), "", ConfigDir{})
	if err != nil || !strings.HasPrefix(out, "CLAUDECODE= JOB= PATH-set=true ") {
		t.Fatalf("out %q, err %v", out, err)
	}
}

// A call for a session of another account reaches that account's daemon only
// with its CLAUDE_CONFIG_DIR, and ~/.claude only with none at all.
func TestCallsToClaudeRunWithTheAccountsConfigDir(t *testing.T) {
	t.Setenv(helperEnv, "env")
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/1-you_example.com")
	for _, tc := range []struct {
		dir  ConfigDir
		want string
	}{
		{ConfigDir{}, "CONFIG=/profiles/1-you_example.com CONFIG-set=true"},
		{ConfigDir{Path: "/profiles/2-work_example.com"}, "CONFIG=/profiles/2-work_example.com CONFIG-set=true"},
		{ConfigDir{Unset: true}, "CONFIG= CONFIG-set=false"},
	} {
		out, _, err := execRunner{Binary: os.Args[0]}.Run(t.Context(), "", tc.dir)
		if err != nil || !strings.HasSuffix(out, tc.want) {
			t.Errorf("%s: out %q, err %v, want %q", tc.dir, out, err, tc.want)
		}
	}
}

func TestEveryCallForASessionPassesItsConfigDirOn(t *testing.T) {
	r := &fakeRunner{stdout: `[{"id":"242e8b07","sessionId":"242e8b07-a8ec-4656-900e-0f340368feb7","kind":"background"}]`}
	c := &Client{Runner: r}
	work := ConfigDir{Path: "/profiles/2-work_example.com"}
	sessions, err := c.Sessions(t.Context(), work, true)
	if err != nil || sessions[0].ConfigDir != work {
		t.Fatalf("sessions %+v, err %v", sessions, err)
	}
	s := sessions[0]
	c.Stop(t.Context(), s.ConfigDir, s.JobID)
	c.Respawn(t.Context(), s.ConfigDir, s.JobID)
	c.Remove(t.Context(), s.ConfigDir, s.JobID)
	if _, err := c.Resume(t.Context(), s.ConfigDir, s.SessionID, t.TempDir(), "", NewJob{}); err != nil {
		t.Fatal(err)
	}
	c.Version(t.Context())
	want := []ConfigDir{work, work, work, work, work, {}}
	if !slices.Equal(r.configs, want) {
		t.Fatalf("configs %v, want %v", r.configs, want)
	}
}

func TestConfigDirReplacesOrRemovesTheVariable(t *testing.T) {
	env := []string{"HOME=/home/me", "CLAUDE_CONFIG_DIR=/a"}
	if got := (ConfigDir{}).apply(env); !slices.Equal(got, env) {
		t.Errorf("own: %q", got)
	}
	if got := (ConfigDir{Path: "/b"}).apply(env); !slices.Equal(got, []string{"HOME=/home/me", "CLAUDE_CONFIG_DIR=/b"}) {
		t.Errorf("set: %q", got)
	}
	if got := (ConfigDir{Unset: true}).apply(env); !slices.Equal(got, []string{"HOME=/home/me"}) {
		t.Errorf("unset: %q", got)
	}
}

func TestManualCommandRunsUnderTheSessionsAccount(t *testing.T) {
	s := Session{SessionID: "sid", Cwd: "/x", Kind: KindInteractive}
	if got := s.ManualCommand(); got != shell.ResumeCommand("sid", "/x", true) {
		t.Errorf("own account: %q", got)
	}
	s.ConfigDir = ConfigDir{Path: "/p"}
	if got := s.ManualCommand(); got != shell.ResumeCommandIn("/p", "sid", "/x", true) {
		t.Errorf("another account: %q", got)
	}
	s.ConfigDir = ConfigDir{Unset: true}
	if got := s.ManualCommand(); got != shell.ResumeCommandIn("", "sid", "/x", true) {
		t.Errorf("~/.claude: %q", got)
	}
}

func TestResumeRefusesAPromptThatLooksLikeAnOption(t *testing.T) {
	r := &fakeRunner{}
	_, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", t.TempDir(), "--dangerously-skip-permissions", NewJob{})
	if err == nil || len(r.calls) != 0 {
		t.Fatalf("err %v, calls %q", err, r.calls)
	}
}

func TestResumeThatStartsACopyIsAFailure(t *testing.T) {
	r := &fakeRunner{stderr: "Session is already running; started a copy as a new session."}
	_, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", t.TempDir(), "", NewJob{})
	if err == nil || !strings.Contains(err.Error(), "started a copy") {
		t.Fatalf("err %v", err)
	}
}

func TestStopRespawnAndRemoveTakeTheJobID(t *testing.T) {
	r := &fakeRunner{}
	c := &Client{Runner: r}
	ctx := t.Context()
	c.Stop(ctx, ConfigDir{}, "242e8b07")
	c.Respawn(ctx, ConfigDir{}, "242e8b07")
	c.Remove(ctx, ConfigDir{}, "242e8b07")
	want := [][]string{{"stop", "242e8b07"}, {"respawn", "242e8b07"}, {"rm", "242e8b07"}}
	for i := range want {
		if !slices.Equal(r.calls[i], want[i]) {
			t.Fatalf("call %d: %q", i, r.calls[i])
		}
	}
}

func TestAFailingCommandReportsWhatItPrinted(t *testing.T) {
	r := &fakeRunner{stderr: "No job matching 'x'\n", err: &exec.ExitError{}}
	got := (&Client{Runner: r}).Stop(t.Context(), ConfigDir{}, "x")
	if got == nil || got.Error() != "claude stop x failed: No job matching 'x'" {
		t.Fatalf("error %v", got)
	}
}

func TestResumeRunsInTheFolderWithThePromptLast(t *testing.T) {
	r := &fakeRunner{}
	dir := t.TempDir()
	if _, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", dir, "carry on", NewJob{}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.calls[0], []string{"--bg", "--resume", "sid", "carry on"}) || r.dirs[0] != dir {
		t.Fatalf("args %q in %q", r.calls[0], r.dirs[0])
	}
}

func TestResumeGivesANewJobItsNameAndOptionsBeforeTheSession(t *testing.T) {
	r := &fakeRunner{}
	job := NewJob{Name: "plan", Options: []string{"--add-dir", "/x", "--model", "m"}}
	if _, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", t.TempDir(), "carry on", job); err != nil {
		t.Fatal(err)
	}
	want := []string{"--bg", "--name=plan", "--add-dir", "/x", "--model", "m", "--resume", "sid", "carry on"}
	if !slices.Equal(r.calls[0], want) {
		t.Fatalf("args %q", r.calls[0])
	}
}

func TestResumeNamesANewJobOnce(t *testing.T) {
	r := &fakeRunner{}
	job := NewJob{Name: "new", Options: []string{"--name=old", "--model", "m", "-n", "older", "--name", "oldest", "-n=x"}}
	if _, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", t.TempDir(), "", job); err != nil {
		t.Fatal(err)
	}
	if want := []string{"--bg", "--name=new", "--model", "m", "--resume", "sid"}; !slices.Equal(r.calls[0], want) {
		t.Fatalf("args %q", r.calls[0])
	}
}

func TestResumeChecksTheFolderFirst(t *testing.T) {
	r := &fakeRunner{}
	_, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", "/no/such/directory", "", NewJob{})
	if err == nil || !strings.Contains(err.Error(), "is gone") || len(r.calls) != 0 {
		t.Fatalf("err %v, calls %q", err, r.calls)
	}
}

func TestVersion(t *testing.T) {
	r := &fakeRunner{stdout: "2.1.325 (Claude Code)\n"}
	v, err := (&Client{Runner: r}).Version(t.Context())
	if err != nil || v != "2.1.325" {
		t.Fatalf("version %q, err %v", v, err)
	}
}

func TestHelpAsksTheRightCommand(t *testing.T) {
	r := &fakeRunner{stdout: "Usage: claude"}
	c := &Client{Runner: r}
	if out, err := c.Help(t.Context(), ""); err != nil || out != "Usage: claude" {
		t.Fatalf("out %q, err %v", out, err)
	}
	if _, err := c.Help(t.Context(), "agents"); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(r.calls); got != "[[--help] [agents --help]]" {
		t.Fatalf("calls %s", got)
	}
}

func TestNewClientRunsClaudeFromPATHWithTimeouts(t *testing.T) {
	c := NewClient()
	r, ok := c.Runner.(execRunner)
	if !ok || r.Binary != "claude" || r.WaitDelay <= 0 || c.ListTimeout <= 0 || c.ActionTimeout <= 0 {
		t.Fatalf("client %+v", c)
	}
}

func TestAFailureSaysWhatClaudePrinted(t *testing.T) {
	for mode, want := range map[string]string{
		"fail-stdout": "claude stop 242e8b07 failed: Not logged in",
		"fail-silent": "claude stop 242e8b07 failed: exit status 3",
	} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(helperEnv, mode)
			c := &Client{Runner: execRunner{Binary: os.Args[0]}}
			if err := c.Stop(t.Context(), ConfigDir{}, "242e8b07"); err == nil || err.Error() != want {
				t.Fatalf("err %v, want %q", err, want)
			}
		})
	}
}

func TestAMissingClaudeIsReportedAsSuch(t *testing.T) {
	c := &Client{Runner: execRunner{Binary: filepath.Join(t.TempDir(), "claude")}}
	_, err := c.Sessions(t.Context(), ConfigDir{}, false)
	if err == nil || !strings.HasPrefix(err.Error(), "cannot run claude: ") {
		t.Fatalf("err %v", err)
	}
}

func TestACanceledCallSaysSo(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := (&Client{Runner: blockingRunner{}}).Respawn(ctx, ConfigDir{}, "242e8b07")
	if err == nil || err.Error() != "claude respawn 242e8b07: context canceled" {
		t.Fatalf("err %v", err)
	}
}

func TestSessionsRejectsOutputThatIsNotAList(t *testing.T) {
	for _, out := range []string{`{"sessions":[]}`, "Please log in", ""} {
		r := &fakeRunner{stdout: out}
		if sessions, err := (&Client{Runner: r}).Sessions(t.Context(), ConfigDir{}, false); err == nil {
			t.Fatalf("%q parsed as %+v", out, sessions)
		}
	}
	r := &fakeRunner{stderr: "boom", err: &exec.ExitError{}}
	if _, err := (&Client{Runner: r}).Sessions(t.Context(), ConfigDir{}, false); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err %v", err)
	}
}

func TestVersionFailures(t *testing.T) {
	if _, err := (&Client{Runner: &fakeRunner{stdout: " \n"}}).Version(t.Context()); err == nil {
		t.Fatal("no error when claude --version printed nothing")
	}
	r := &fakeRunner{stderr: "boom", err: &exec.ExitError{}}
	if _, err := (&Client{Runner: r}).Version(t.Context()); err == nil {
		t.Fatal("no error when claude --version failed")
	}
}

func TestLabelAndAliases(t *testing.T) {
	bg := Session{JobID: "242e8b07", SessionID: "16cf5295-0000-0000-0000-000000000000"}
	if bg.Label() != "242e8b07" {
		t.Fatalf("label %q", bg.Label())
	}
	if got := bg.Aliases(); !slices.Equal(got, []string{"242e8b07", bg.SessionID, "16cf5295"}) {
		t.Fatalf("aliases %q", got)
	}
	named := Session{Name: "db", SessionID: "abc"}
	if got := named.Aliases(); !slices.Equal(got, []string{"db", "abc", "abc"}) {
		t.Fatalf("aliases %q", got)
	}
}

func TestCheckPromptRefusesAnOptionAndAsksTheRunner(t *testing.T) {
	c := &Client{Runner: &fakeRunner{}}
	if err := c.CheckPrompt("--model x"); err == nil || !strings.Contains(err.Error(), "read it as an option") {
		t.Fatalf("err %v", err)
	}
	if err := c.CheckPrompt("carry on"); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("cannot pass it")
	c = &Client{Runner: &fakeRunner{checkErr: refused}}
	if err := c.CheckPrompt("a & b"); !errors.Is(err, refused) {
		t.Fatalf("err %v, want the runner's", err)
	}
}

func TestEachCallIsLoggedWhenThereIsALog(t *testing.T) {
	var buf strings.Builder
	c := &Client{Runner: &fakeRunner{stdout: "[]"}, Log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	if _, err := c.Sessions(t.Context(), ConfigDir{}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `args="agents --json --all"`) {
		t.Fatalf("log %q", buf.String())
	}
}

func TestResumeKeepsANameThatStartsWithADashOneArgument(t *testing.T) {
	r := &fakeRunner{}
	if _, err := (&Client{Runner: r}).Resume(t.Context(), ConfigDir{}, "sid", t.TempDir(), "", NewJob{Name: "-x"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(r.calls[0], "--name=-x") {
		t.Fatalf("args %q", r.calls[0])
	}
}

func TestWithoutSessionVarsKeepsTheUsersOwnSettings(t *testing.T) {
	env := []string{
		"CLAUDE_CODE_CHILD_NOTES_DIR=/tmp/notes",
		"CLAUDE_EFFORT=high",
		"CLAUDE_CODE_SESSION_ID=abc",
		"CLAUDE_CODE_SCROLL_SPEED=3",
		"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1",
		"HOME=/home/me",
	}
	got := withoutSessionVars(env)
	want := []string{"CLAUDE_CODE_SCROLL_SPEED=3", "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1", "HOME=/home/me"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
