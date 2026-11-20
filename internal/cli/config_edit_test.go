package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/config"
)

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
