package cli

import (
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
