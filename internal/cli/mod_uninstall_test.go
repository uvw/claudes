package cli

import (
	"os"
	"testing"
)

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
