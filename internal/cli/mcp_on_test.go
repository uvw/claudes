package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mcp"
	"github.com/uvw/claudes/internal/mod"
)

// The ways the fake mod answers a request.
const (
	modRuns    = "runs"    // runs /mcp, which changes ~/.claude.json
	modNoop    = "noop"    // says done, but nothing changes
	modRefuses = "refuses" // refuses every request
	modSilent  = "silent"  // never answers
)

// fakeMod stands in for the claudes mod in s, answering its requests the way
// how says each time claudes sleeps. It returns the requests it saw.
func (h *harness) fakeMod(t *testing.T, s claude.Session, how string) *[]mod.Request {
	t.Helper()
	h.agents.markets = []claude.Marketplace{{Name: mod.Marketplace, Source: "github", Repo: mod.DefaultSource}}
	h.agents.plugins = []claude.Plugin{{ID: mod.ID, Version: "0.1.0", Scope: "user", Enabled: true}}
	h.loaded(t, s, "0.1.0")
	var seen []mod.Request
	sleep := h.app.Sleep
	h.app.Sleep = func(ctx context.Context, d time.Duration) error {
		data, err := os.ReadFile(filepath.Join(h.app.Mod.Dir, "requests", s.SessionID+".json"))
		if err == nil && how != modSilent {
			var file struct {
				Requests []mod.Request `json:"requests"`
			}
			if err := json.Unmarshal(data, &file); err != nil {
				t.Fatal(err)
			}
			for _, r := range file.Requests {
				if slices.Contains(seen, r) {
					continue
				}
				seen = append(seen, r)
				res := mod.Result{ID: r.ID, Session: s.SessionID, Status: mod.Done, Version: "0.1.0"}
				switch how {
				case modRefuses:
					res.Status, res.Reason = mod.Refused, "the mod runs only /mcp"
				case modRuns:
					verb, name, _ := strings.Cut(r.Args, " ")
					if _, _, err := h.app.MCP.SetDisabled(s.Cwd, func(list []string) []string {
						list = slices.DeleteFunc(list, func(n string) bool { return n == name })
						if verb == "disable" {
							list = append(list, name)
						}
						return list
					}); err != nil {
						t.Fatal(err)
					}
				}
				out, _ := json.Marshal(res)
				writeFile(t, filepath.Join(h.app.Mod.Dir, "results", r.ID+".json"), string(out))
			}
		}
		return sleep(ctx, d)
	}
	return &seen
}

// mcpSession is a running session whose directory has the servers browser
// and web-api, with web-api turned off.
func (h *harness) mcpSession(t *testing.T, status string) claude.Session {
	t.Helper()
	s := h.bg("api", status)
	h.agents.snapshots = [][]claude.Session{{s}}
	h.mcpServers(t, s.Cwd, map[string]any{"disabledMcpServers": []string{"web-api"}})
	return s
}

func (h *harness) disabled(t *testing.T, dir string) []string {
	t.Helper()
	d, err := h.app.MCP.Disabled(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestMCPOffAsksTheModAndChecksTheChange(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	seen := h.fakeMod(t, s, modRuns)

	h.ok(t, "mcp", "off", "browser", "api")

	if len(*seen) != 1 || (*seen)[0].Command != "mcp" || (*seen)[0].Args != "disable browser" {
		t.Fatalf("requests %+v", *seen)
	}
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"web-api", "browser"}) {
		t.Fatalf("disabled %q", got)
	}
	assertContains(t, h.out.String(), "through its claudes mod")
	assertContains(t, h.out.String(), "turned off")
	noSecrets(t, h.out.String())
	for _, sub := range []string{"requests", "results"} {
		if entries, _ := os.ReadDir(filepath.Join(h.app.Mod.Dir, sub)); len(entries) != 0 {
			t.Fatalf("%s left behind: %v", sub, entries)
		}
	}
}

func TestMCPOnTurnsOnByPatternAndLeavesWhatIsAlreadyOn(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	seen := h.fakeMod(t, s, modRuns)

	h.ok(t, "mcp", "on", "--all", "api")

	if len(*seen) != 1 || (*seen)[0].Args != "enable web-api" {
		t.Fatalf("requests %+v", *seen)
	}
	if got := h.disabled(t, s.Cwd); len(got) != 0 {
		t.Fatalf("disabled %q", got)
	}
	assertContains(t, h.out.String(), "already on")

	h.out.Reset()
	*seen = nil
	h.ok(t, "mcp", "off", "w*", "-o", "json", "api")
	var set mcpSet
	if err := json.Unmarshal(h.out.Bytes(), &set); err != nil {
		t.Fatalf("%v\n%s", err, h.out)
	}
	if set.Via != viaMod || set.Dir != s.Cwd || set.ID != s.JobID || len(set.Servers) != 1 ||
		set.Servers[0] != (mcpChange{Name: "web-api", From: mcp.On, To: mcp.Off, Result: setDone, request: set.Servers[0].request}) {
		t.Fatalf("%+v", set)
	}
}

func TestMCPOffSendsNothingWhenEveryServerIsAlreadyOff(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	seen := h.fakeMod(t, s, modRuns)
	h.ok(t, "mcp", "off", "web-api", "api")
	if len(*seen) != 0 || len(h.sleeps) != 0 {
		t.Fatalf("asked the mod: %+v", *seen)
	}
	assertContains(t, h.out.String(), "already off")
}

func TestMCPOffRefusesAPatternThatMatchesNoServer(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	h.fakeMod(t, s, modRuns)
	h.fails(t, "mcp", "off", "nope*", "api")
	assertContains(t, h.err.String(), "Servers: browser, web-api")
	h.fails(t, "mcp", "off", "api")
	assertContains(t, h.err.String(), "--all")
	h.fails(t, "mcp", "off", "--all", "browser", "api")
	h.fails(t, "mcp", "off", "[", "api")
}

func TestMCPOffWithoutTheModSaysHowToGetIt(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")

	h.exits(t, exitRefused, "mcp", "off", "browser", "api")
	assertContains(t, h.err.String(), "claudes mod install")
	assertContains(t, h.err.String(), "--restart")

	h.fakeMod(t, s, modRuns)
	if err := os.Remove(filepath.Join(h.app.Mod.Dir, "sessions", s.SessionID+".json")); err != nil {
		t.Fatal(err)
	}
	h.exits(t, exitRefused, "mcp", "off", "browser", "api")
	assertContains(t, h.err.String(), "/reload-plugins")

	h.agents.plugins[0].Enabled = false
	h.exits(t, exitRefused, "mcp", "off", "browser", "api")
	assertContains(t, h.err.String(), "claude plugin enable")
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"web-api"}) {
		t.Fatalf("changed the list: %q", got)
	}
}

func TestMCPOffReportsAModThatDoesNotAnswerRefusesOrChangesNothing(t *testing.T) {
	for how, want := range map[string]string{modSilent: setTimedOut, modRefuses: setRefused, modNoop: setUnchanged} {
		t.Run(how, func(t *testing.T) {
			h := newHarness(t)
			s := h.mcpSession(t, "busy")
			h.fakeMod(t, s, how)

			h.exits(t, exitSomeFailed, "mcp", "off", "-o", "json", "--timeout", "10s", "browser", "api")

			var set mcpSet
			if err := json.Unmarshal(h.out.Bytes(), &set); err != nil {
				t.Fatalf("%v\n%s", err, h.out)
			}
			if len(set.Servers) != 1 || set.Servers[0].Result != want || set.Servers[0].Note == "" {
				t.Fatalf("%+v", set.Servers)
			}
			if _, err := os.Stat(filepath.Join(h.app.Mod.Dir, "requests", s.SessionID+".json")); !os.IsNotExist(err) {
				t.Fatalf("left the request behind: %v", err)
			}
		})
	}
}

func TestMCPOffInTheSessionRunningClaudesQueuesAndReturns(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "busy")
	h.host(s)
	seen := h.fakeMod(t, s, modSilent)

	h.ok(t, "mcp", "off", "browser", "api")

	assertContains(t, h.out.String(), "runs when this turn ends")
	if len(h.sleeps) != 0 || len(*seen) != 0 {
		t.Fatalf("waited for its own turn: %v", h.sleeps)
	}
	if _, err := os.Stat(filepath.Join(h.app.Mod.Dir, "requests", s.SessionID+".json")); err != nil {
		t.Fatalf("no request for the mod: %v", err)
	}
}

func TestMCPOffWithRestartWritesTheListWhileTheSessionIsDown(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	var atStop []string
	h.agents.beforeStop = func(string) { atStop = h.disabled(t, s.Cwd) }

	h.ok(t, "mcp", "off", "--restart", "browser", "api")

	if !slices.Equal(atStop, []string{"web-api"}) {
		t.Fatalf("wrote before the stop: %q", atStop)
	}
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"web-api", "browser"}) {
		t.Fatalf("disabled %q", got)
	}
	assertCalls(t, h, "stop "+s.JobID, "resume "+s.SessionID+" in api prompt=")
	assertContains(t, h.out.String(), "by restarting it")
	if entries, _ := h.app.Park.Load(); len(entries) != 0 {
		t.Fatalf("left it parked: %+v", entries)
	}
}

func TestMCPOffWithRestartTakesAnInteractiveSessionOnlyWhenAsked(t *testing.T) {
	h := newHarness(t)
	s := h.tty("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.mcpServers(t, s.Cwd, map[string]any{})

	h.exits(t, exitRefused, "mcp", "off", "--restart", "browser", "api")
	assertContains(t, h.err.String(), "--interactive")
	if len(h.terminated) != 0 {
		t.Fatal("stopped it")
	}

	h.ok(t, "mcp", "off", "--restart", "--interactive", "browser", "api")
	if !slices.Equal(h.terminated, []int{s.PID}) || !slices.Equal(h.disabled(t, s.Cwd), []string{"browser"}) {
		t.Fatalf("terminated %v, disabled %q", h.terminated, h.disabled(t, s.Cwd))
	}
	assertContains(t, h.out.String(), "claude --resume "+s.SessionID)
}

func TestMCPOffWithRestartInTheSessionRunningClaudesWritesFirst(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "busy")
	h.host(s)
	h.agents.beforeRespawn = func(string) {
		if !slices.Contains(h.disabled(t, s.Cwd), "browser") {
			t.Fatal("respawned before writing")
		}
	}
	h.ok(t, "mcp", "off", "--restart", "browser", "api")
	assertCalls(t, h, "respawn "+s.JobID)
}

func TestMCPOffInADirectoryWritesTheListUnlessASessionRunsThere(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")

	h.exits(t, exitRefused, "mcp", "off", "--in", s.Cwd, "browser")
	assertContains(t, h.err.String(), "name it instead")

	h.agents.snapshots = [][]claude.Session{{stopped(s)}}
	h.ok(t, "mcp", "off", "--in", s.Cwd, "browser")
	if got := h.disabled(t, s.Cwd); !slices.Equal(got, []string{"web-api", "browser"}) {
		t.Fatalf("disabled %q", got)
	}
	assertContains(t, h.out.String(), "turned off")

	h.fails(t, "mcp", "off", "--in", h.folder("never-opened"), "browser")
	assertContains(t, h.err.String(), "start a Claude Code session there once")

	h.fails(t, "mcp", "off", "--in", s.Cwd, "--restart", "browser")
}

func TestMCPOffDryRunChangesNothing(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	seen := h.fakeMod(t, s, modRuns)
	for _, extra := range []string{"", "--restart"} {
		args := []string{"mcp", "off", "--dry-run", "browser", "api"}
		if extra != "" {
			args = append(args, extra)
		}
		h.out.Reset()
		h.ok(t, args...)
		assertContains(t, h.out.String(), "planned off")
		assertContains(t, h.out.String(), "Dry run")
	}
	if len(*seen) != 0 || len(h.agents.calls) != 0 || !slices.Equal(h.disabled(t, s.Cwd), []string{"web-api"}) {
		t.Fatalf("changed something: %+v %q", *seen, h.agents.calls)
	}
}

func TestMCPOffRefusesASessionThatIsNotRunning(t *testing.T) {
	h := newHarness(t)
	s := h.mcpSession(t, "idle")
	h.agents.snapshots = [][]claude.Session{{stopped(s)}}
	h.exits(t, exitRefused, "mcp", "off", "browser", "api")
	h.exits(t, exitNoMatch, "mcp", "off", "browser", "nothing-like-it")
}
