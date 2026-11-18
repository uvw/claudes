package cli

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestConfigSetThenGet(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "# mine\nping:\n  text: carry on\n")
	h.ok(t, "config", "set", "wait.timeout", "45m")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# mine") || !strings.Contains(string(data), "timeout: 45m") {
		t.Fatalf("set lost the comment or the value:\n%s", data)
	}
	h.out.Reset()
	if code := h.run("config", "get", "wait.timeout"); code != 0 || strings.TrimSpace(h.out.String()) != "45m0s" {
		t.Fatalf("get: exit %d, %q", code, h.out)
	}
	h.out.Reset()
	h.ok(t, "config", "get")
	for _, want := range []string{"ping.text", "carry on", "output.format", "-"} {
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("get all lacks %q:\n%s", want, h.out)
		}
	}
}

func TestConfigSetRefusesBadValues(t *testing.T) {
	h := newHarness(t)
	path := h.withConfig(t, "")
	for _, args := range [][]string{
		{"output.format", "xml"},
		{"wait.timeout", "soon"},
		{"ping.text", "--dangerously-skip-permissions"},
		{"nope.key", "1"},
	} {
		if code := h.run(append([]string{"config", "set"}, args...)...); code == 0 {
			t.Errorf("set %v: exit 0, want a refusal", args)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused set wrote %s", path)
	}
}
