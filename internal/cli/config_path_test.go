package cli

import (
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	if code := h.run("config", "path"); code != 0 || strings.TrimSpace(h.out.String()) != path {
		t.Fatalf("exit %d, %q, want %s", code, h.out, path)
	}
}
