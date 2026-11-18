package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mod"
)

// checkout makes a directory that holds the claudes marketplace, as a clone of
// the repository does.
func (h *harness) checkout(t *testing.T) string {
	t.Helper()
	dir := h.folder("claudes-checkout")
	writeFile(t, filepath.Join(dir, ".claude-plugin", "marketplace.json"), `{"name":"claudes"}`)
	return dir
}

// loaded writes the presence file the mod writes when s loads it.
func (h *harness) loaded(t *testing.T, s claude.Session, version string) {
	t.Helper()
	if err := h.app.Mod.Prepare(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(mod.Presence{Protocol: mod.Protocol, Session: s.SessionID, Version: version, At: h.now.UnixMilli()})
	writeFile(t, filepath.Join(h.app.Mod.Dir, "sessions", s.SessionID+".json"), string(data))
}

func TestModInstallAddsTheMarketplaceFromACheckoutAndInstalls(t *testing.T) {
	h := newHarness(t)
	dir := h.checkout(t)

	h.ok(t, "mod", "install", "--from", dir)

	assertCalls(t, h, "marketplace add "+dir, "plugin install claudes-mod@claudes")
	assertContains(t, h.out.String(), "/reload-plugins")
	info, err := os.Stat(h.app.Mod.Dir)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("channel directory: %v %v", info, err)
	}

	h.agents.calls = nil
	h.out.Reset()
	h.ok(t, "mod", "install", "--from", dir)
	assertCalls(t, h)
	assertContains(t, h.out.String(), "already installed")
}

func TestModInstallDefaultsToTheGitHubRepository(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "mod", "install")
	assertCalls(t, h, "marketplace add uvw/claudes", "plugin install claudes-mod@claudes")
}

func TestModInstallRefusesAMarketplaceFromElsewhere(t *testing.T) {
	h := newHarness(t)
	dir := h.checkout(t)
	h.agents.markets = []claude.Marketplace{{Name: "claudes", Source: "github", Repo: "uvw/claudes"}}

	h.fails(t, "mod", "install", "--from", dir)

	assertCalls(t, h)
	assertContains(t, h.err.String(), "claudes mod uninstall")
}

func TestModInstallRefusesADirectoryWithNoMarketplace(t *testing.T) {
	h := newHarness(t)
	h.fails(t, "mod", "install", "--from", h.folder("not-claudes"))
	assertCalls(t, h)
	assertContains(t, h.err.String(), "marketplace.json")
}

func TestModInstallSaysWhenTheModIsTurnedOff(t *testing.T) {
	h := newHarness(t)
	h.agents.markets = []claude.Marketplace{{Name: "claudes", Source: "github", Repo: "uvw/claudes"}}
	h.agents.plugins = []claude.Plugin{{ID: mod.ID, Version: "0.1.0", Enabled: false}}
	h.ok(t, "mod", "install")
	assertCalls(t, h)
	assertContains(t, h.out.String(), "claude plugin enable claudes-mod@claudes")
}

func TestModUninstallRemovesThePluginTheMarketplaceAndTheChannel(t *testing.T) {
	h := newHarness(t)
	h.ok(t, "mod", "install", "--from", h.checkout(t))
	h.agents.calls = nil

	h.ok(t, "mod", "uninstall")

	assertCalls(t, h, "plugin uninstall claudes-mod@claudes", "marketplace remove claudes")
	if _, err := os.Stat(h.app.Mod.Dir); !os.IsNotExist(err) {
		t.Fatalf("channel directory is still there: %v", err)
	}
	h.out.Reset()
	h.ok(t, "mod", "uninstall")
	assertContains(t, h.out.String(), "not installed")
}

func TestModStatusListsWhichRunningSessionsLoadedIt(t *testing.T) {
	h := newHarness(t)
	with, without, done := h.bg("with-mod", "idle"), h.tty("without-mod", "idle"), stopped(h.bg("finished", ""))
	h.agents.snapshots = [][]claude.Session{{with, without, done}}
	h.ok(t, "mod", "install", "--from", h.checkout(t))
	h.loaded(t, with, "0.1.0")

	h.out.Reset()
	h.ok(t, "mod", "status")
	text := h.out.String()
	assertContains(t, text, "claudes-mod 0.1.0 is installed from")
	assertContains(t, text, "ID ")
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.Contains(line, "with-mod") && !strings.Contains(line, "without-mod"):
			assertContains(t, line, "0.1.0")
		case strings.Contains(line, "without-mod"):
			assertContains(t, line, "not loaded")
		case strings.Contains(line, "finished"):
			t.Fatalf("listed a session that is not running: %q", line)
		}
	}

	h.out.Reset()
	h.ok(t, "mod", "status", "-o", "json")
	var st modState
	if err := json.Unmarshal(h.out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.Installed || !st.Enabled || len(st.Sessions) != 2 || st.Sessions[0].Version != "0.1.0" || st.Sessions[1].Version != "" {
		t.Fatalf("%+v", st)
	}
}

func TestModStatusIgnoresAPresenceFromBeforeTheSessionStarted(t *testing.T) {
	h := newHarness(t)
	s := h.bg("restarted", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	h.now = h.now.Add(-2 * time.Hour)
	h.loaded(t, s, "0.1.0")
	h.now = h.now.Add(2 * time.Hour)
	if got := h.app.modLoaded(s); got != "" {
		t.Fatalf("took an old presence as loaded: %q", got)
	}
}

func TestDoctorChecksTheMod(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}

	if c := h.app.checkMod(t.Context()); c.Result != resultOK || !strings.Contains(c.Detail, "not installed") {
		t.Fatalf("%+v", c)
	}
	h.ok(t, "mod", "install")
	if c := h.app.checkMod(t.Context()); c.Result != resultWarn || !strings.Contains(c.Detail, "/reload-plugins in api") {
		t.Fatalf("%+v", c)
	}
	h.loaded(t, s, "0.1.0")
	if c := h.app.checkMod(t.Context()); c.Result != resultOK {
		t.Fatalf("%+v", c)
	}
	h.agents.plugins[0].Enabled = false
	if c := h.app.checkMod(t.Context()); c.Result != resultWarn || !strings.Contains(c.Detail, "turned off") {
		t.Fatalf("%+v", c)
	}
	h.agents.failures = map[string]string{"plugin list": "boom"}
	if c := h.app.checkMod(t.Context()); c.Result != resultWarn {
		t.Fatalf("%+v", c)
	}
}
