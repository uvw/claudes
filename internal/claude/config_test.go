package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// configFixture is a made-up home directory with a ~/.claude.json and a work
// directory.
type configFixture struct {
	t    *testing.T
	root string
}

func newConfigFixture(t *testing.T) *configFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &configFixture{t: t, root: root}
}

func (f *configFixture) path() string { return filepath.Join(f.root, ".claude.json") }

func (f *configFixture) dir(parts ...string) string {
	f.t.Helper()
	d := filepath.Join(append([]string{f.root, "work"}, parts...)...)
	if err := os.MkdirAll(d, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return d
}

func (f *configFixture) write(path, body string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// projects writes ~/.claude.json with an entry for each directory, which
// names the directory it was written for.
func (f *configFixture) projects(dirs ...string) Config {
	f.t.Helper()
	projects := map[string]any{}
	for _, d := range dirs {
		projects[filepath.ToSlash(d)] = map[string]any{"for": d}
	}
	data, err := json.Marshal(map[string]any{"projects": projects})
	if err != nil {
		f.t.Fatal(err)
	}
	f.write(f.path(), string(data))
	cfg, err := ReadConfig(f.path())
	if err != nil {
		f.t.Fatal(err)
	}
	return cfg
}

// entry is the directory the entry Project finds for dir was written for, or
// "" for none.
func entry(t *testing.T, cfg Config, dir string) string {
	t.Helper()
	var d string
	if raw, ok := cfg.Project(dir)["for"]; ok {
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatal(err)
		}
	}
	return d
}

func TestReadConfig(t *testing.T) {
	f := newConfigFixture(t)
	for _, path := range []string{"", "relative.json", filepath.Join(f.root, "missing.json")} {
		cfg, err := ReadConfig(path)
		if err != nil || cfg.Key("projects") != nil {
			t.Errorf("%q: %v, %v", path, cfg, err)
		}
	}
	f.write(f.path(), `{"numStartups": 3}`)
	if cfg, err := ReadConfig(f.path()); err != nil || string(cfg.Key("numStartups")) != "3" {
		t.Errorf("numStartups: %s, %v", cfg.Key("numStartups"), err)
	}
	f.write(f.path(), `["not", "an", "object"]`)
	if _, err := ReadConfig(f.path()); err == nil {
		t.Error("a file that is not a JSON object gave no error")
	}
	if _, err := ReadConfig(f.root); err == nil {
		t.Error("a directory gave no error")
	}
}

func TestAWorktreeUsesItsMainRepositorysEntry(t *testing.T) {
	f := newConfigFixture(t)
	main := f.dir("repo")
	sub := f.dir("repo", "pkg")
	tree := f.dir("repo", ".claude", "worktrees", "feature")
	gitDir := filepath.Join(main, ".git", "worktrees", "feature")
	f.write(filepath.Join(gitDir, "commondir"), "../..\n")
	f.write(filepath.Join(tree, ".git"), "gitdir: "+gitDir+"\n")
	cfg := f.projects(main)
	for _, d := range []string{main, sub, tree} {
		if got := entry(t, cfg, d); got != main {
			t.Errorf("%s has the entry for %q", d, got)
		}
	}

	// A submodule's .git file has no commondir, so it is its own repository.
	module := f.dir("repo", "vendor", "lib")
	f.write(filepath.Join(main, ".git", "modules", "lib", "HEAD"), "ref: refs/heads/main\n")
	f.write(filepath.Join(module, ".git"), "gitdir: ../../.git/modules/lib\n")
	if got := entry(t, cfg, module); got != "" {
		t.Errorf("the submodule has the entry for %q", got)
	}

	// A .git file that names no git directory makes its directory the root.
	odd := f.dir("repo", "odd")
	f.write(filepath.Join(odd, ".git"), "not a gitdir line\n")
	if got := entry(t, f.projects(main, odd), odd); got != odd {
		t.Errorf("odd has the entry for %q", got)
	}
}

func TestALinkIsFollowedBeforeTheRepositoryIsFound(t *testing.T) {
	f := newConfigFixture(t)
	main := f.dir("repo")
	f.write(filepath.Join(main, ".git", "HEAD"), "ref: refs/heads/main\n")
	inside := f.dir("repo", "jobs", "scratch")
	link := filepath.Join(f.root, "link")
	if err := os.Symlink(filepath.Join(main, "jobs"), link); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	cfg := f.projects(main, filepath.Join(link, "scratch"))
	for _, d := range []string{filepath.Join(link, "scratch"), inside} {
		if got := entry(t, cfg, d); got != main {
			t.Errorf("%s has the entry for %q", d, got)
		}
	}
}

func TestOutsideARepositoryTheDirectoryIsItsOwnEntry(t *testing.T) {
	f := newConfigFixture(t)
	dir := f.dir("plain")
	if got := entry(t, f.projects(dir), dir); got != dir {
		t.Errorf("%s has the entry for %q", dir, got)
	}
	if got := entry(t, f.projects(), dir); got != "" {
		t.Errorf("with no projects, %s has the entry for %q", dir, got)
	}
}

func TestFeatureReadsTheFlagsClaudeCodeCaches(t *testing.T) {
	f := newConfigFixture(t)
	const flag = "tengu_hover_rest"
	for _, tc := range []struct {
		body      string
		on, found bool
	}{
		{`{"cachedGrowthBookFeatures":{"tengu_hover_rest":false}}`, false, true},
		{`{"cachedGrowthBookFeatures":{"tengu_hover_rest":true}}`, true, true},
		{`{"cachedGrowthBookFeatures":{"tengu_hover_rest":{"variant":"a"}}}`, true, true},
		{`{"cachedGrowthBookFeatures":{"tengu_hover_rest":null}}`, false, true},
		{`{"cachedGrowthBookFeatures":{"other":true}}`, false, false},
		{`{"numStartups":3}`, false, false},
	} {
		f.write(f.path(), tc.body)
		on, found, err := Feature(f.path(), flag)
		if err != nil || on != tc.on || found != tc.found {
			t.Errorf("%s: on %v, found %v, err %v", tc.body, on, found, err)
		}
	}
	if _, found, err := Feature(filepath.Join(f.root, "missing.json"), flag); found || err != nil {
		t.Errorf("a missing file: found %v, err %v", found, err)
	}
	if _, found, err := Feature("", flag); found || err != nil {
		t.Errorf("no file: found %v, err %v", found, err)
	}
	f.write(f.path(), "not json")
	if _, _, err := Feature(f.path(), flag); err == nil {
		t.Error("a file that is not JSON gave no error")
	}
}
