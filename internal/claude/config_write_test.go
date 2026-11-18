package claude

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// claudeJSON is a ~/.claude.json the way Claude Code writes it: through
// JSON.stringify(config, null, 2), with no newline at the end.
const claudeJSON = `{
  "numStartups": 12,
  "mcpServers": {
    "browser": {
      "command": "/bin/sk-command-secret",
      "env": {
        "TOKEN": "sk-env-secret"
      }
    }
  },
  "projects": {
    "/src/api": {
      "allowedTools": [],
      "disabledMcpServers": [
        "browser",
        "web"
      ],
      "lastCost": 0.25
    },
    "/src/empty": {},
    "/src/fresh": {
      "allowedTools": [],
      "note": "café \"quoted\" <tag> & more"
    }
  },
  "userID": "abc"
}`

// setup writes text as ~/.claude.json in a temporary home, with project
// directories that exist, and returns the file's path and the home.
func setup(t *testing.T, text string) (string, string) {
	t.Helper()
	home := t.TempDir()
	root, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"api", "empty", "fresh", "other"} {
		if err := os.MkdirAll(filepath.Join(root, "src", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	text = strings.ReplaceAll(text, `"/src/`, `"`+filepath.ToSlash(root)+`/src/`)
	file := filepath.Join(root, ".claude.json")
	if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return file, root
}

func contents(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func set(list ...string) func([]string) []string {
	return func([]string) []string { return list }
}

func TestSetDisabledReplacesOnlyTheList(t *testing.T) {
	for _, tc := range []struct {
		name, dir string
		list      []string
		old, new  string
	}{
		{"a longer list", "api", []string{"browser", "web", "plugin:docs:search"},
			"      \"disabledMcpServers\": [\n        \"browser\",\n        \"web\"\n      ],",
			"      \"disabledMcpServers\": [\n        \"browser\",\n        \"web\",\n        \"plugin:docs:search\"\n      ],"},
		{"an empty list", "api", nil,
			"      \"disabledMcpServers\": [\n        \"browser\",\n        \"web\"\n      ],",
			"      \"disabledMcpServers\": [],"},
		{"a list the entry lacks", "fresh", []string{"browser"},
			"      \"note\": \"café \\\"quoted\\\" <tag> & more\"\n    }",
			"      \"note\": \"café \\\"quoted\\\" <tag> & more\",\n      \"disabledMcpServers\": [\n        \"browser\"\n      ]\n    }"},
		{"an empty entry", "empty", []string{"browser"},
			"\"/src/empty\": {},",
			"\"/src/empty\": {\n      \"disabledMcpServers\": [\n        \"browser\"\n      ]\n    },"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, root := setup(t, claudeJSON)
			before, after, err := SetDisabled(file, filepath.Join(root, "src", tc.dir), set(tc.list...))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(after, append([]string{}, tc.list...)) {
				t.Fatalf("after %q", after)
			}
			if tc.dir == "api" && !slices.Equal(before, []string{"browser", "web"}) {
				t.Fatalf("before %q", before)
			}
			old := strings.ReplaceAll(tc.old, `"/src/`, `"`+filepath.ToSlash(root)+`/src/`)
			want := strings.ReplaceAll(strings.ReplaceAll(claudeJSON, `"/src/`, `"`+filepath.ToSlash(root)+`/src/`), old,
				strings.ReplaceAll(tc.new, `"/src/`, `"`+filepath.ToSlash(root)+`/src/`))
			if got := contents(t, file); got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestSetDisabledKeepsAOneLineFileOnOneLine(t *testing.T) {
	file, root := setup(t, `{"projects":{"/src/api":{"disabledMcpServers":["a"]},"/src/fresh":{"x":1}}}`)
	if _, _, err := SetDisabled(file, filepath.Join(root, "src", "api"), set("a", "b")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SetDisabled(file, filepath.Join(root, "src", "fresh"), set("c")); err != nil {
		t.Fatal(err)
	}
	p := filepath.ToSlash(root)
	want := `{"projects":{"` + p + `/src/api":{"disabledMcpServers":["a","b"]},"` + p + `/src/fresh":{"x":1,"disabledMcpServers":["c"]}}}`
	if got := contents(t, file); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestSetDisabledFindsTheEntryByTheRepositoryRoot(t *testing.T) {
	file, root := setup(t, claudeJSON)
	if err := os.MkdirAll(filepath.Join(root, "src", "api", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src", "api", "cmd")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadConfig(file)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := cfg.EntryKey(sub); got != filepath.ToSlash(filepath.Join(root, "src", "api")) {
		t.Fatalf("key %q", got)
	}
	if _, _, err := SetDisabled(file, sub, set()); err != nil {
		t.Fatal(err)
	}
	if cfg, err = ReadConfig(file); err != nil || len(cfg.Disabled(sub)) != 0 {
		t.Fatalf("disabled %q, %v", cfg.Disabled(sub), err)
	}
	if !strings.Contains(contents(t, file), `"disabledMcpServers": [],`) {
		t.Fatal(contents(t, file))
	}
}

func TestSetDisabledWritesNothingWhenNothingChanges(t *testing.T) {
	file, root := setup(t, claudeJSON)
	info, _ := os.Stat(file)
	before, after, err := SetDisabled(file, filepath.Join(root, "src", "api"), set("browser", "web"))
	if err != nil || !slices.Equal(before, after) {
		t.Fatalf("%q %q %v", before, after, err)
	}
	if now, _ := os.Stat(file); !now.ModTime().Equal(info.ModTime()) {
		t.Fatal("rewrote the file")
	}
}

func TestSetDisabledRefusesADirectoryWithNoEntry(t *testing.T) {
	file, root := setup(t, claudeJSON)
	_, _, err := SetDisabled(file, filepath.Join(root, "src", "other"), set("x"))
	if !errors.Is(err, ErrNoProject) {
		t.Fatalf("got %v", err)
	}
	if contents(t, file) == "" {
		t.Fatal("emptied the file")
	}
}

func TestSetDisabledStartsAgainWhenClaudeCodeWritesMeanwhile(t *testing.T) {
	file, root := setup(t, claudeJSON)
	calls := 0
	_, after, err := SetDisabled(file, filepath.Join(root, "src", "api"), func(current []string) []string {
		calls++
		if calls == 1 {
			// Claude Code saves the file between claudes' read and its write.
			text := strings.Replace(contents(t, file), `"numStartups": 12`, `"numStartups": 13`, 1)
			if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return append(current, "late")
	})
	if err != nil || calls != 2 || !slices.Equal(after, []string{"browser", "web", "late"}) {
		t.Fatalf("calls %d, after %q, err %v", calls, after, err)
	}
	text := contents(t, file)
	if !strings.Contains(text, `"numStartups": 13`) || !strings.Contains(text, `"late"`) {
		t.Fatalf("lost a write:\n%s", text)
	}
}

func TestSetDisabledGivesUpOnAFileThatKeepsChanging(t *testing.T) {
	file, root := setup(t, claudeJSON)
	n := 0
	_, _, err := SetDisabled(file, filepath.Join(root, "src", "api"), func(current []string) []string {
		n++
		text := strings.Replace(contents(t, file), `"userID": "`, `"userID": "x`, 1)
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	if !errors.Is(err, errChanged) || n != 3 {
		t.Fatalf("got %v after %d tries", err, n)
	}
}

func TestSetDisabledKeepsTheModeAndTheLink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("links need privileges on Windows, and modes are not POSIX")
	}
	file, root := setup(t, claudeJSON)
	real := filepath.Join(root, "dotfiles.json")
	if err := os.Rename(file, real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, file); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SetDisabled(file, filepath.Join(root, "src", "api"), set("only")); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(file); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link became a file: %v", err)
	}
	if info, _ := os.Stat(real); info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
	if !strings.Contains(contents(t, real), `"only"`) {
		t.Fatal("did not write through the link")
	}
}

func TestSetDisabledRefusesAFileThatIsNotJSON(t *testing.T) {
	file, root := setup(t, `{"projects": `)
	if _, _, err := SetDisabled(file, filepath.Join(root, "src", "api"), set("x")); err == nil {
		t.Fatal("wrote into a broken file")
	}
}

func TestScannerHandlesEscapesAndNesting(t *testing.T) {
	text := `{"a\"}": [1, {"b": "]}"}], "c": {"d": [true, null, -1.5e3]}, "e": "x"}`
	sc := scanner{data: []byte(text)}
	obj, err := sc.object(0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range obj.members {
		names = append(names, m.name)
	}
	if !slices.Equal(names, []string{`a"}`, "c", "e"}) || obj.end != len(text) {
		t.Fatalf("names %q end %d", names, obj.end)
	}
	c, _ := obj.member("c")
	if text[c.value:c.end] != `{"d": [true, null, -1.5e3]}` {
		t.Fatalf("value %q", text[c.value:c.end])
	}
}
