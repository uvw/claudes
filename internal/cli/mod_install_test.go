package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/mod"
)

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

func TestSourceTakesACheckoutOrPassesTheRestThrough(t *testing.T) {
	dir := t.TempDir()
	if got, err := modSource(""); err != nil || got != mod.DefaultSource {
		t.Fatalf("got %q, %v", got, err)
	}
	if got, err := modSource("owner/repo"); err != nil || got != "owner/repo" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := modSource(dir); err == nil {
		t.Fatal("took a directory with no marketplace")
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(dir)
	if got, err := modSource(dir); err != nil || got != real {
		t.Fatalf("got %q, %v", got, err)
	}
}
