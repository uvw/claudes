package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

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

func TestConfigSetsFlagDefaults(t *testing.T) {
	h := newHarness(t)
	h.withConfig(t, "ping:\n  text: keep going\nwait:\n  timeout: 5m\noutput:\n  format: json\n")
	root := h.app.rootCommand()
	// The commands docs/reference.md says follow output.format, and two it
	// says stay text.
	for _, c := range []struct{ cmd, flag, want string }{
		{"ls", "output", "json"},
		{"where", "output", "json"},
		{"team ls", "output", "json"},
		{"session ls", "output", "json"},
		{"session show", "output", "json"},
		{"session where", "output", "json"},
		{"session du", "output", "json"},
		{"session search", "output", "json"},
		{"session triage", "output", "json"},
		{"doctor", "output", "json"},
		{"doctor env", "output", "json"},
		{"mcp ls", "output", "json"},
		{"mcp profiles", "output", "json"},
		{"logs", "output", "text"},
		{"restart", "output", "text"},
		{"restart", "timeout", (5 * time.Minute).String()},
		{"pause", "timeout", (5 * time.Minute).String()},
	} {
		cmd, _, err := root.Find(strings.Fields(c.cmd))
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
