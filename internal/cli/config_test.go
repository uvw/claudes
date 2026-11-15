package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/config"
)

// withConfig points h at a config.yaml in its temporary folder, loaded the way
// newApp loads it.
func (h *harness) withConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(h.root, "config", "config.yaml")
	if body != "" {
		writeFile(t, path, body)
	}
	h.app.ConfigPath = path
	h.app.Config, h.app.configErr = config.Load(path)
	return path
}

func TestConfigSetThenGet(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "# mine\nping:\n  text: carry on\n")
	h.ok(t, "config", "set", "wait.timeout", "45m")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# mine") || !strings.Contains(string(data), "timeout: 45m") {
		t.Fatalf("set lost the comment or the value:\n%s", data)
	}
	h.out.Reset()
	if code := h.run("config", "get", "wait.timeout"); code != 0 || strings.TrimSpace(h.out.String()) != "45m0s" {
		t.Fatalf("get: exit %d, %q", code, h.out)
	}
	h.out.Reset()
	h.ok(t, "config", "get")
	for _, want := range []string{"ping.text", "carry on", "output.format", "-"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("get all lacks %q:\n%s", want, h.out)
		}
	}
}

func TestConfigSetRefusesBadValues(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	for _, args := range [][]string{
		{"output.format", "xml"},
		{"wait.timeout", "soon"},
		{"ping.text", "--dangerously-skip-permissions"},
		{"nope.key", "1"},
	} {
		if code := h.run(append([]string{"config", "set"}, args...)...); code == 0 {
			t.Errorf("set %v: exit 0, want a refusal", args)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused set wrote %s", path)
	}
}

func TestConfigPath(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	if code := h.run("config", "path"); code != 0 || strings.TrimSpace(h.out.String()) != path {
		t.Fatalf("exit %d, %q, want %s", code, h.out, path)
	}
}

func TestBrokenConfigStopsCommandsButConfig(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "output:\n  fromat: json\n")
	if code := h.run("ls"); code != 1 || !strings.Contains(h.err.String(), "claudes config edit") {
		t.Fatalf("ls: exit %d, %q", code, h.err)
	}
	if len(h.agents.calls) > 0 {
		t.Fatalf("ls reached claude with a broken config: %v", h.agents.calls)
	}
	h.err.Reset()
	if code := h.run("-v", "ls"); code != 1 || !strings.Contains(h.err.String(), "claudes config edit") || len(h.agents.calls) > 0 {
		t.Fatalf("-v ls: exit %d, %q, calls %v", code, h.err, h.agents.calls)
	}
	h.err.Reset()
	if code := h.run("config", "get"); code != 1 || !strings.Contains(h.err.String(), "fromat") {
		t.Fatalf("config get: exit %d, %q", code, h.err)
	}
	h.ok(t, "config", "path")
	h.ok(t, "--help")
}

func TestConfigEditStartsFromTemplate(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	var opened string
	h.app.Edit = func(p string) error {
		opened = p
		return os.WriteFile(p, []byte("output:\n  format: yaml\n"), 0o600)
	}
	h.ok(t, "config", "edit")
	if opened != path {
		t.Fatalf("opened %q, want %q", opened, path)
	}
}

func TestConfigEditWritesTemplateAndChecksTheResult(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	var before []byte
	h.app.Edit = func(p string) error {
		var err error
		if before, err = os.ReadFile(p); err != nil {
			return err
		}
		return os.WriteFile(p, []byte("bogus: 1\n"), 0o600)
	}
	if code := h.run("config", "edit"); code != 1 || !strings.Contains(h.err.String(), "cannot use it") {
		t.Fatalf("exit %d, %q", code, h.err)
	}
	if string(before) != string(config.Template()) {
		t.Fatalf("the editor did not open the template:\n%s", before)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetsFlagDefaults(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "ping:\n  text: keep going\nwait:\n  timeout: 5m\noutput:\n  format: json\n")
	root := h.app.rootCommand()
	for _, c := range []struct{ cmd, flag, want string }{
		{"ls", "output", "json"},
		{"where", "output", "json"},
		{"restart", "timeout", (5 * time.Minute).String()},
		{"pause", "timeout", (5 * time.Minute).String()},
	} {
		cmd, _, err := root.Find([]string{c.cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := cmd.Flags().Lookup(c.flag).DefValue; got != c.want {
			t.Errorf("%s --%s defaults to %q, want %q", c.cmd, c.flag, got, c.want)
		}
	}
}

func TestConfigPingTextIsWhatPingSends(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "ping:\n  text: keep going\n")
	var f lifecycleFlags
	f.addPing(&cobra.Command{}, h.app.pingPrompt(defaultPing))
	f.ping = true
	if f.text() != "keep going" {
		t.Fatalf("--ping sends %q", f.text())
	}
	f.pingText = "x"
	if f.text() != "x" {
		t.Fatal("--ping-text lost to the config")
	}
}

func TestABrokenConfigLeavesShellSetupAlone(t *testing.T) {
	for _, args := range [][]string{{"init", "zsh"}, {"completion", "bash"}, {"--version"}, {"skill"}} {
		h := newHarness(t)
		h.withConfig(t, "output:\n  fromat: json\n")
		if code := h.run(args...); code != 0 {
			t.Errorf("%v: exit %d, %s", args, code, h.err)
		}
	}
}

func TestPingWithoutAMessageSendsPingText(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "ping:\n  text: keep going\n")
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	h.ok(t, "ping", "api")
	assertCalls(t, h, "stop "+s.JobID, "resume "+s.SessionID+" in api prompt=keep going")
}
