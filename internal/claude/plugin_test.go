package claude

import (
	"slices"
	"strings"
	"testing"
)

func TestPluginsKeepsNoServerDetails(t *testing.T) {
	r := &fakeRunner{stdout: `[{"id":"claudes-mod@claudes","version":"0.1.0","scope":"user","enabled":true,
		"installPath":"/p","mcpServers":{"x":{"url":"https://example.com/sk-url-secret"}}}]`}
	c := &Client{Runner: r}
	got, err := c.Plugins(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []Plugin{{ID: "claudes-mod@claudes", Version: "0.1.0", Scope: "user", Enabled: true, InstallPath: "/p"}}
	if !slices.Equal(got, want) || !slices.Equal(r.calls[0], []string{"plugin", "list", "--json"}) {
		t.Fatalf("got %+v, calls %q", got, r.calls)
	}
}

func TestMarketplacesSayWhereEachComesFrom(t *testing.T) {
	r := &fakeRunner{stdout: `[{"name":"a","source":"github","repo":"uvw/claudes"},
		{"name":"b","source":"directory","path":"/src/claudes"},{"name":"c","source":"url","url":"https://x/m.json"}]`}
	c := &Client{Runner: r}
	got, err := c.Marketplaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var where []string
	for _, m := range got {
		where = append(where, m.Where())
	}
	if !slices.Equal(where, []string{"uvw/claudes", "/src/claudes", "https://x/m.json"}) {
		t.Fatalf("got %q", where)
	}
}

func TestPluginListsThatAreNotListsFail(t *testing.T) {
	c := &Client{Runner: &fakeRunner{stdout: `{"error":"x"}`}}
	if _, err := c.Plugins(t.Context()); err == nil || !strings.Contains(err.Error(), "plugin list") {
		t.Fatalf("got %v", err)
	}
	if _, err := c.Marketplaces(t.Context()); err == nil || !strings.Contains(err.Error(), "marketplace list") {
		t.Fatalf("got %v", err)
	}
}

func TestPluginCommands(t *testing.T) {
	r := &fakeRunner{}
	c := &Client{Runner: r}
	ctx := t.Context()
	for _, err := range []error{
		c.AddMarketplace(ctx, "/src/claudes"),
		c.InstallPlugin(ctx, "claudes-mod@claudes"),
		c.UninstallPlugin(ctx, "claudes-mod@claudes"),
		c.RemoveMarketplace(ctx, "claudes"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := [][]string{
		{"plugin", "marketplace", "add", "/src/claudes"},
		{"plugin", "install", "claudes-mod@claudes"},
		{"plugin", "uninstall", "claudes-mod@claudes"},
		{"plugin", "marketplace", "remove", "claudes"},
	}
	for i := range want {
		if !slices.Equal(r.calls[i], want[i]) {
			t.Fatalf("call %d: %q", i, r.calls[i])
		}
	}
}
