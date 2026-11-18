package cli

import (
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
)

func TestDoctorColorsEachResult(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("odd", "thinking")}}
	h.agents.version = "2.9.0"
	h.env("CLICOLOR_FORCE=1")

	h.fails(t, "doctor")

	out := h.out.String()
	for _, want := range []string{h.app.colors.Green("ok"), h.app.colors.Yellow("warn"), h.app.colors.Red("fail")} {
		assertContains(t, out, want)
	}
	assertContains(t, h.err.String(), h.app.errColors.Red("1 check(s) failed."))
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
	h.loaded(t, s)
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
