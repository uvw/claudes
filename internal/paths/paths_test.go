package paths

import (
	"path/filepath"
	"testing"
)

func TestXDGVariablesWinWhenAbsolute(t *testing.T) {
	base := t.TempDir()
	for variable, get := range map[string]func() (string, error){
		"XDG_CONFIG_HOME": Config,
		"XDG_STATE_HOME":  State,
		"XDG_CACHE_HOME":  Cache,
	} {
		t.Setenv(variable, base)
		if got, err := get(); err != nil || got != filepath.Join(base, "claudes") {
			t.Errorf("%s: got %q, err %v", variable, got, err)
		}
		t.Setenv(variable, "relative")
		if got, err := get(); err != nil || got == filepath.Join("relative", "claudes") || !filepath.IsAbs(got) {
			t.Errorf("%s relative: got %q, err %v", variable, got, err)
		}
	}
}

func TestTheFoldersDifferAndEndInClaudes(t *testing.T) {
	for _, v := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(v, "")
	}
	seen := map[string]bool{}
	for _, get := range []func() (string, error){Config, State, Cache} {
		got, err := get()
		if err != nil {
			t.Fatal(err)
		}
		if seen[got] {
			t.Fatalf("%q used twice", got)
		}
		seen[got] = true
	}
}

func TestClaudeHome(t *testing.T) {
	abs := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", abs)
	if got, err := ClaudeHome(); err != nil || got != abs {
		t.Fatalf("got %q, err %v", got, err)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "relative")
	if got, err := ClaudeHome(); err == nil {
		t.Fatalf("accepted a relative CLAUDE_CONFIG_DIR: %q", got)
	}
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got, err := ClaudeHome(); err != nil || got != filepath.Join(home, ".claude") {
		t.Fatalf("got %q, err %v", got, err)
	}
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	if got, err := ClaudeHome(); err == nil {
		t.Fatalf("guessed %q with no home directory", got)
	}
}
