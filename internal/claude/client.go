package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Agents is what claudes needs from the claude command line.
type Agents interface {
	// Sessions lists sessions; all adds finished background sessions.
	Sessions(ctx context.Context, all bool) ([]Session, error)
	Respawn(ctx context.Context, jobID string) error
	Stop(ctx context.Context, jobID string) error
	Remove(ctx context.Context, jobID string) error
	// Resume continues a session in the background from its directory, with an
	// optional prompt, and returns what the command printed. job is for a
	// session with no background job yet, and empty otherwise: an existing job
	// wakes with its own saved options, and any option passed then starts a
	// copy.
	Resume(ctx context.Context, sessionID, cwd, prompt string, job NewJob) (string, error)
	// Version is the installed Claude Code version, such as "2.1.325".
	Version(ctx context.Context) (string, error)
	// Help is the output of `claude --help`, or of `claude COMMAND --help`.
	Help(ctx context.Context, command string) (string, error)
	// CheckPrompt refuses a prompt that claude would not receive as one
	// argument, so a command can refuse it before it stops any session.
	CheckPrompt(prompt string) error
}

// NewJob is what claude --bg --resume gets for a session that has no
// background job, which Claude Code would otherwise start with no name and
// none of the options the session ran with.
type NewJob struct {
	Name string
	// Options are command-line options, such as --model, as Claude Code
	// saves them for a job.
	Options []string
}

// Runner runs the claude binary and captures its output.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error)
	// Check refuses arguments that would not reach the binary intact.
	Check(args ...string) error
}

// execRunner runs a real binary.
type execRunner struct {
	Binary string
	// WaitDelay bounds the wait for output once the binary has exited. A
	// background session that claude starts can inherit its stdout and hold
	// it open for as long as the session runs.
	WaitDelay time.Duration
}

// Check implements Runner. A binary that is not on PATH passes, since running
// it fails with a clearer error.
func (r execRunner) Check(args ...string) error {
	path, err := exec.LookPath(r.Binary)
	if err != nil {
		return nil
	}
	return checkArgs(path, args)
}

// Run implements Runner.
func (r execRunner) Run(ctx context.Context, dir string, args ...string) (string, string, error) {
	path, err := exec.LookPath(r.Binary)
	if err != nil {
		return "", "", err
	}
	if err := checkArgs(path, args); err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.Env = childEnv()
	cmd.WaitDelay = r.WaitDelay
	isolate(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if errors.Is(err, exec.ErrWaitDelay) {
		// The binary exited cleanly; only a child kept the pipe open.
		err = nil
	}
	return stdout.String(), stderr.String(), err
}

// sessionVars mark a process as running inside a Claude Code session. A
// session that claudes starts or resumes is not a child of the session claudes
// runs in, so they are left out of its environment.
var sessionVars = []string{
	"AI_AGENT",
	"CLAUDECODE",
	"CLAUDE_CODE_CHILD_SESSION",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_MESSAGING_SOCKET",
	"CLAUDE_CODE_MESSAGING_TOKEN",
	"CLAUDE_CODE_SESSION_ATTENDED",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_JOB_DIR",
	"CLAUDE_PID",
}

// childEnv is the environment claude runs with: claudes' own, without the
// session variables, plus platformEnv.
func childEnv() []string { return append(withoutSessionVars(os.Environ()), platformEnv...) }

func withoutSessionVars(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.ContainsFunc(sessionVars, func(v string) bool { return strings.EqualFold(v, name) }) {
			out = append(out, kv)
		}
	}
	return out
}

// Client implements Agents with the claude command line.
type Client struct {
	Runner Runner
	// ListTimeout bounds `claude agents --json`, `--version`, and `rm`.
	// ActionTimeout bounds stop, respawn, and resume, which can wait on a
	// session shutting down. Zero means no limit.
	ListTimeout, ActionTimeout time.Duration
	// Log, when set, gets each call at debug level, with its time and result.
	Log *slog.Logger
}

// NewClient returns a Client that runs `claude` from PATH.
func NewClient() *Client {
	return &Client{
		Runner:        execRunner{Binary: "claude", WaitDelay: 5 * time.Second},
		ListTimeout:   time.Minute,
		ActionTimeout: 3 * time.Minute,
	}
}

func (c *Client) run(ctx context.Context, timeout time.Duration, label, dir string, args ...string) (string, string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := time.Now()
	stdout, stderr, err := c.Runner.Run(ctx, dir, args...)
	if c.Log != nil {
		c.Log.Debug("claude", "args", strings.Join(args, " "), "dir", dir,
			"took", time.Since(start).Round(time.Millisecond), "err", err)
	}
	if err == nil {
		return stdout, stderr, nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", "", fmt.Errorf("%s timed out after %s", label, timeout)
	}
	if ctx.Err() != nil {
		return "", "", fmt.Errorf("%s: %w", label, ctx.Err())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return "", "", fmt.Errorf("cannot run claude: %w", err)
	}
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = strings.TrimSpace(stdout)
	}
	if detail == "" {
		detail = exitErr.Error()
	}
	return "", "", fmt.Errorf("%s failed: %s", label, detail)
}

// Sessions implements Agents.
func (c *Client) Sessions(ctx context.Context, all bool) ([]Session, error) {
	args := []string{"agents", "--json"}
	if all {
		args = append(args, "--all")
	}
	out, _, err := c.run(ctx, c.ListTimeout, "claude agents --json", "", args...)
	if err != nil {
		return nil, err
	}
	return parseSessions([]byte(out))
}

// parseSessions decodes the output of `claude agents --json`. An entry with no
// session id is left out: nothing claudes does can act on it, and one such
// entry must not hide every other session.
func parseSessions(data []byte) ([]Session, error) {
	var sessions []Session
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("claude agents --json returned something other than a list: %w", err)
	}
	return slices.DeleteFunc(sessions, func(s Session) bool { return s.SessionID == "" }), nil
}

// Respawn implements Agents.
func (c *Client) Respawn(ctx context.Context, jobID string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude respawn "+jobID, "", "respawn", jobID)
	return err
}

// Stop implements Agents.
func (c *Client) Stop(ctx context.Context, jobID string) error {
	_, _, err := c.run(ctx, c.ActionTimeout, "claude stop "+jobID, "", "stop", jobID)
	return err
}

// Remove implements Agents.
func (c *Client) Remove(ctx context.Context, jobID string) error {
	_, _, err := c.run(ctx, c.ListTimeout, "claude rm "+jobID, "", "rm", jobID)
	return err
}

// CheckPrompt implements Agents.
func (c *Client) CheckPrompt(prompt string) error {
	if err := CheckOption(prompt); err != nil {
		return err
	}
	return c.Runner.Check(prompt)
}

// CheckOption refuses a prompt that claude would read as an option.
func CheckOption(prompt string) error {
	if strings.HasPrefix(prompt, "-") {
		return fmt.Errorf("the prompt %q starts with -, so claude would read it as an option", prompt)
	}
	return nil
}

// copyNotice matches what `claude --bg --resume` prints when the session is
// still running: `claude --help` says it "starts a copy and says so".
var copyNotice = regexp.MustCompile(`(?i)\ba copy\b`)

// Resume implements Agents.
func (c *Client) Resume(ctx context.Context, sessionID, cwd, prompt string, job NewJob) (string, error) {
	// Run checks the arguments against the binary as well.
	if err := CheckOption(prompt); err != nil {
		return "", err
	}
	if !FolderExists(cwd) {
		return "", fmt.Errorf("directory %s is gone", cwd)
	}
	args := []string{"--bg"}
	options := job.Options
	if job.Name != "" {
		args = append(args, "--name="+job.Name)
		// Claude Code saves --name with a job's options, so a saved one
		// would name the job twice.
		options = withoutName(options)
	}
	// The options go before --resume, which ends an option that takes a list,
	// such as --add-dir, before it can take the prompt.
	args = append(args, options...)
	args = append(args, "--resume", sessionID)
	if prompt != "" {
		args = append(args, prompt)
	}
	label := "claude --bg --resume " + ShortID(sessionID)
	stdout, stderr, err := c.run(ctx, c.ActionTimeout, label, cwd, args...)
	if err != nil {
		return "", err
	}
	if copyNotice.MatchString(stdout) || copyNotice.MatchString(stderr) {
		return stdout, fmt.Errorf("%s started a copy, because the session was still running: %s",
			label, strings.TrimSpace(stdout+" "+stderr))
	}
	return stdout, nil
}

// withoutName is options without --name or -n and its value.
func withoutName(options []string) []string {
	var out []string
	for i := 0; i < len(options); i++ {
		o := options[i]
		switch {
		case o == "--name" || o == "-n":
			i++
		case strings.HasPrefix(o, "--name=") || strings.HasPrefix(o, "-n="):
		default:
			out = append(out, o)
		}
	}
	return out
}

// FolderExists reports whether dir is an existing directory.
func FolderExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// Help implements Agents.
func (c *Client) Help(ctx context.Context, command string) (string, error) {
	args := []string{"--help"}
	if command != "" {
		args = []string{command, "--help"}
	}
	out, _, err := c.run(ctx, c.ListTimeout, "claude "+strings.Join(args, " "), "", args...)
	return out, err
}

// Version implements Agents.
func (c *Client) Version(ctx context.Context) (string, error) {
	out, _, err := c.run(ctx, c.ListTimeout, "claude --version", "", "--version")
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", errors.New("claude --version printed nothing")
	}
	return fields[0], nil
}
