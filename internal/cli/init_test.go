package cli

import "testing"

func TestInitAndCd(t *testing.T) {
	h := newHarness(t)
	for shell, want := range map[string]string{
		"zsh":  "command claudes session where --one",
		"fish": "and cd -- $dir",
		"pwsh": "Set-Location -LiteralPath $dir",
	} {
		h.out.Reset()
		h.ok(t, "init", shell)
		assertContains(t, h.out.String(), want)
	}
	h.fails(t, "init", "tcsh")
	h.fails(t, "cd", "plan")
	assertContains(t, h.err.String(), "claudes init pwsh")
}
