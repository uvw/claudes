// Command fakeclaude stands in for the claude command line in the end-to-end
// tests. It keeps its sessions in the JSON file FAKE_CLAUDE_STATE and appends
// every call to FAKE_CLAUDE_LOG, one JSON object per line.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/uvw/claudes/test/e2e/internal/fake"
)

// state is fake.State with the fake's behavior.
type state fake.State

var jobID = regexp.MustCompile(`^[0-9a-f]{8}$`)

func main() {
	statePath, logPath := os.Getenv("FAKE_CLAUDE_STATE"), os.Getenv("FAKE_CLAUDE_LOG")
	if statePath == "" || logPath == "" {
		fmt.Fprintln(os.Stderr, "fakeclaude: FAKE_CLAUDE_STATE and FAKE_CLAUDE_LOG must be set")
		os.Exit(2)
	}
	args := os.Args[1:]
	logCall(logPath, args)
	st, err := load(statePath)
	if err != nil {
		fail(err.Error())
	}
	code := st.handle(args)
	if err := save(statePath, st); err != nil {
		fail(err.Error())
	}
	os.Exit(code)
}

func (st *state) handle(args []string) int {
	if len(args) == 0 {
		fmt.Println("fakeclaude: interactive mode is not faked")
		return 2
	}
	if msg, ok := st.Fail[args[0]]; ok {
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}
	switch args[0] {
	case "--version":
		fmt.Printf("%s (Claude Code)\n", st.Version)
		return 0
	case "--help":
		fmt.Print(rootHelp)
		return 0
	case "agents":
		if len(args) == 2 && args[1] == "--help" {
			fmt.Print(agentsHelp)
			return 0
		}
		if len(args) > 1 && args[1] == "--json" {
			return st.list(slices.Contains(args[2:], "--all"))
		}
		fmt.Printf("agents %s\n", strings.Join(args[1:], " "))
		return st.AgentsExit
	case "stop", "respawn", "rm", "logs", "attach":
		if len(args) != 2 {
			fmt.Fprintf(os.Stderr, "usage: claude %s <id>\n", args[0])
			return 1
		}
		return st.job(args[0], args[1])
	case "--resume":
		if len(args) >= 2 && len(args) <= 3 {
			fmt.Printf("resumed %s here\n", args[1])
			return 0
		}
	case "--bg":
		// --bg [--name=NAME] [OPTION...] --resume ID [PROMPT]
		if i := slices.Index(args, "--resume"); i > 0 && len(args)-i >= 2 && len(args)-i <= 3 {
			return st.resume(args[i+1], args[1:i])
		}
	}
	fmt.Fprintf(os.Stderr, "fakeclaude: unknown call %q\n", args)
	return 2
}

func (st *state) list(all bool) int {
	out := []fake.Session{}
	for _, s := range st.Sessions {
		if all || s.PID > 0 {
			out = append(out, s)
		}
	}
	data, _ := json.Marshal(out)
	fmt.Println(string(data))
	return 0
}

func (st *state) job(op, id string) int {
	i := slices.IndexFunc(st.Sessions, func(s fake.Session) bool { return s.ID == id })
	if !jobID.MatchString(id) || i < 0 {
		fmt.Fprintf(os.Stderr, "No job matching '%s'\n", id)
		return 1
	}
	s := &st.Sessions[i]
	switch op {
	case "stop":
		s.PID, s.State, s.Status = 0, "stopped", ""
	case "respawn":
		st.start(s)
	case "rm":
		st.Sessions = slices.Delete(st.Sessions, i, i+1)
		if err := os.RemoveAll(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "jobs", id)); err != nil {
			fail(err.Error())
		}
	default:
		fmt.Printf("%s of %s\n", op, s.Name)
	}
	return 0
}

// resume wakes a session's job, or starts a job in this directory for a
// session that has a transcript and no job, with the name and options given.
func (st *state) resume(sessionID string, options []string) int {
	i := slices.IndexFunc(st.Sessions, func(s fake.Session) bool { return s.SessionID == sessionID })
	if i < 0 {
		found, _ := filepath.Glob(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects", "*", sessionID+".jsonl"))
		if len(found) == 0 {
			fmt.Fprintf(os.Stderr, "No conversation found with session ID: %s\n", sessionID)
			return 1
		}
		if err := st.newJob(sessionID, options); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		i = len(st.Sessions) - 1
	} else if len(options) > 0 {
		fmt.Println("The session has a job, so this started a copy of it.")
		return 0
	}
	s := &st.Sessions[i]
	if s.PID > 0 {
		// What the real one does, by `claude --help`: it starts a copy and says so.
		fmt.Println("Session is still running, so this started a copy of it.")
		return 0
	}
	st.start(s)
	fmt.Printf("Resumed %s in the background.\n", s.SessionID)
	return 0
}

// newJob adds a job for a session, and writes its state.json with the options,
// --name included, as Claude Code saves them to pass again.
func (st *state) newJob(sessionID string, options []string) error {
	home := os.Getenv("CLAUDE_CONFIG_DIR")
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	s := fake.Session{ID: sessionID[:8], SessionID: sessionID, Cwd: wd, Kind: "background"}
	for _, o := range options {
		if name, ok := strings.CutPrefix(o, "--name="); ok {
			s.Name = name
		}
	}
	state, err := json.Marshal(map[string]any{"sessionId": sessionID, "cwd": wd, "respawnFlags": options})
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "jobs", s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	st.Sessions = append(st.Sessions, s)
	return os.WriteFile(filepath.Join(dir, "state.json"), state, 0o644)
}

func (st *state) start(s *fake.Session) {
	// PIDs above any real pid_max, so nothing ever signals a real process.
	if st.NextPID == 0 {
		st.NextPID = 9_000_000
	}
	st.NextPID++
	s.PID, s.State, s.Status = st.NextPID, "done", "idle"
}

func logCall(path string, args []string) {
	wd, _ := os.Getwd()
	_, cc := os.LookupEnv("CLAUDECODE")
	_, jd := os.LookupEnv("CLAUDE_JOB_DIR")
	line, _ := json.Marshal(fake.Call{Args: args, Cwd: wd, ClaudeCode: cc, ClaudeJobDir: jd})
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fail(err.Error())
	}
	_, werr := f.Write(append(line, '\n'))
	// A failed close can lose the line as surely as a failed write.
	if err := errors.Join(werr, f.Close()); err != nil {
		fail(err.Error())
	}
}

func load(path string) (*state, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var st fake.State
	return (*state)(&st), json.Unmarshal(data, &st)
}

func save(path string, st *state) error {
	data, err := json.MarshalIndent((*fake.State)(st), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "fakeclaude:", msg)
	os.Exit(2)
}

// rootHelp and agentsHelp hold the lines of the real help that claudes doctor
// looks for.
const (
	rootHelp = `Usage: claude [options] [command] [prompt]

Options:
  --bg, --background                    Start the session in the background
  -r, --resume [value]                  Resume a conversation by session ID
  -v, --version                         Output the version number

Commands:
  agents [options]                      Manage background agents
  attach <id|name>                      Open a background session
  logs <id|name>                        Print a background session's output
  plugin|plugins                        Manage Claude Code plugins
  respawn [options] [id]                Restart a background session
  rm <id>                               Remove a background session
  stop|kill <id>                        Stop a background session
`
	agentsHelp = `Usage: claude agents [options]

Options:
  --all                                 With --json: also include completed
  --json                                Print active sessions as JSON
`
)
