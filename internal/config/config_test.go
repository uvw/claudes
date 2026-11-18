package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/uvw/claudes/internal/render"
)

func write(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAMissingFileIsAnEmptyConfig(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "none.yaml"))
	if err != nil || !cmp.Equal(c, Config{}) {
		t.Fatalf("config %+v, err %v", c, err)
	}
}

func TestLoadReadsEverySetting(t *testing.T) {
	c, err := Load(write(t, "ping:\n  text: carry on\nwait:\n  timeout: 5m\noutput:\n  format: yaml\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Ping: Ping{Text: "carry on"}, Wait: Wait{Timeout: Duration(5 * time.Minute)}, Output: Output{Format: "yaml"}}
	if diff := cmp.Diff(want, c); diff != "" {
		t.Fatalf("config (-want +got):\n%s", diff)
	}
}

func TestLoadRefusesWhatClaudesCannotUse(t *testing.T) {
	for data, want := range map[string]string{
		"ping:\n  txt: typo\n":      "field txt not found",
		"output:\n  format: xml\n":  "output.format must be",
		"output:\n  dirs: short\n":  "output.dirs must be middle, fish, or full, not \"short\"",
		"ping:\n  text: --yolo\n":   "starts with -",
		"wait:\n  timeout: soon\n":  "not a duration",
		"wait:\n  timeout: -1m\n":   "cannot be negative",
		"ping: [not, a, mapping]\n": "cannot construct",
	} {
		if _, err := Load(write(t, data)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err %v, want %q", data, err, want)
		}
	}
}

func TestSetKeepsCommentsAndOtherSettings(t *testing.T) {
	path := write(t, "# mine\nping:\n  text: carry on # keep going\n")
	if err := Set(path, "wait.timeout", "10m"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "ping.text", "pick up"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# mine") {
		t.Fatalf("lost the comment:\n%s", data)
	}
	c, err := Load(path)
	if err != nil || c.Ping.Text != "pick up" || time.Duration(c.Wait.Timeout) != 10*time.Minute {
		t.Fatalf("config %+v, err %v\n%s", c, err, data)
	}
}

func TestSetRefusesABadValueAndAnUnknownKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := Set(path, "output.format", "xml"); err == nil {
		t.Fatal("accepted xml")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("wrote a bad config")
	}
	if err := Set(path, "nope.key", "x"); err == nil || !strings.Contains(err.Error(), "Settings: ping.text") {
		t.Fatalf("err %v", err)
	}
	if err := Set(path, "output.format", "json"); err != nil {
		t.Fatal(err)
	}
	if v, _ := Get(mustLoad(t, path), "output.format"); v != "json" {
		t.Fatalf("got %q", v)
	}
}

func TestSetOutputDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Set(path, "output.dirs", "short"); err == nil || !strings.Contains(err.Error(), "middle, fish, or full") {
		t.Fatalf("err %v, want the allowed values", err)
	}
	for _, v := range render.DirsValues {
		if err := Set(path, "output.dirs", v); err != nil {
			t.Fatal(err)
		}
		if got, _ := Get(mustLoad(t, path), "output.dirs"); got != v {
			t.Fatalf("got %q, want %q", got, v)
		}
	}
	if err := Set(path, "output.format", "json"); err != nil {
		t.Fatal(err)
	}
	if c := mustLoad(t, path); c.Output.Format != "json" || c.Output.Dirs != "full" {
		t.Fatalf("output %+v, want both settings kept", c.Output)
	}
}

func TestTheTemplateNamesEachSectionOnce(t *testing.T) {
	if n := strings.Count(string(Template()), "# output:\n"); n != 1 {
		t.Fatalf("output: is in the template %d times:\n%s", n, Template())
	}
}

func TestTheTemplateLoadsAsAnEmptyConfig(t *testing.T) {
	c, err := Load(write(t, string(Template())))
	if err != nil || !cmp.Equal(c, Config{}) {
		t.Fatalf("config %+v, err %v", c, err)
	}
	for _, k := range Keys {
		if !strings.Contains(string(Template()), strings.SplitN(k.Name, ".", 2)[1]) {
			t.Errorf("template lacks %s", k.Name)
		}
	}
}

func TestPathHonorsClaudesConfig(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "mine.yaml")
	t.Setenv("CLAUDES_CONFIG", abs)
	if p, err := Path(); err != nil || p != abs {
		t.Fatalf("path %q, err %v", p, err)
	}
	t.Setenv("CLAUDES_CONFIG", "relative.yaml")
	if _, err := Path(); err == nil {
		t.Fatal("accepted a relative CLAUDES_CONFIG")
	}
	t.Setenv("CLAUDES_CONFIG", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if p, err := Path(); err != nil || p != filepath.Join(xdg, "claudes", "config.yaml") {
		t.Fatalf("path %q, err %v", p, err)
	}
}

func mustLoad(t *testing.T, path string) Config {
	t.Helper()
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSetOnTheTemplateKeepsItsComments(t *testing.T) {
	path := write(t, string(Template()))
	if err := Set(path, "ping.text", "hi"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "wait.timeout", "5m"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), string(Template())) {
		t.Fatalf("the template's comments are gone:\n%s", data)
	}
	c, err := Load(path)
	if err != nil || c.Ping.Text != "hi" || time.Duration(c.Wait.Timeout) != 5*time.Minute {
		t.Fatalf("config %+v, err %v\n%s", c, err, data)
	}
}

func TestSetOnAnEmptyDocument(t *testing.T) {
	for _, body := range []string{"---\n", "~\n", "null\n", "# just a note\n---\n"} {
		path := write(t, body)
		if err := Set(path, "ping.text", "hi"); err != nil {
			t.Fatalf("%q: %v", body, err)
		}
		if c, err := Load(path); err != nil || c.Ping.Text != "hi" {
			data, _ := os.ReadFile(path)
			t.Fatalf("%q: config %+v, err %v\n%s", body, c, err, data)
		}
	}
}

func TestSetRefusesAFileThatIsNotAMapping(t *testing.T) {
	path := write(t, "- a\n- b\n")
	if err := Set(path, "ping.text", "hi"); err == nil || !strings.Contains(err.Error(), "no list of settings") {
		t.Fatalf("err %v", err)
	}
}

func TestSeveralDocumentsAreRefused(t *testing.T) {
	path := write(t, "ping:\n  text: hi\n---\nwait:\n  timeout: 5m\n")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "more than one YAML document") {
		t.Fatalf("Load: %v", err)
	}
	if err := Set(path, "output.format", "json"); err == nil {
		t.Fatal("Set dropped the second document")
	}
}

func TestANullDurationIsNoSetting(t *testing.T) {
	if c := mustLoad(t, write(t, "wait:\n  timeout: ~\n")); c.Wait.Timeout != 0 {
		t.Fatalf("timeout %v", c.Wait.Timeout)
	}
}

func TestSetKeepsAValueThatLooksLikeAnotherType(t *testing.T) {
	path := write(t, "")
	for _, v := range []string{"null", "30", "true"} {
		if err := Set(path, "ping.text", v); err != nil {
			t.Fatal(err)
		}
		if c := mustLoad(t, path); c.Ping.Text != v {
			t.Fatalf("set %q, read %q", v, c.Ping.Text)
		}
	}
}

func TestSetWritesThroughALink(t *testing.T) {
	real := write(t, "# mine\n")
	link := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot make a link here: %v", err)
	}
	if err := Set(link, "ping.text", "hi"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v", err)
	}
	if c := mustLoad(t, real); c.Ping.Text != "hi" {
		t.Fatalf("the linked file holds %+v", c)
	}
}

func TestMCPProfiles(t *testing.T) {
	path := write(t, "mcp:\n  profiles:\n    safe: [browser, \"plugin:docs:search\"]\n    none:\n")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]ServerNames{"safe": {"browser", "plugin:docs:search"}, "none": nil}
	if diff := cmp.Diff(want, c.MCP.Profiles); diff != "" {
		t.Fatalf("profiles (-want +got):\n%s", diff)
	}
	if v, _ := Get(c, "mcp.profiles"); v != `{"none":[],"safe":["browser","plugin:docs:search"]}` {
		t.Fatalf("get: %s", v)
	}
	if v, _ := Get(Config{}, "mcp.profiles"); v != "" {
		t.Fatalf("get with no profiles: %q", v)
	}
	if err := Set(path, "mcp.profiles", "safe"); err == nil || !strings.Contains(err.Error(), "config edit") {
		t.Fatalf("set: %v", err)
	}
}

func TestMCPProfilesRefuseWhatIsNotANameList(t *testing.T) {
	for data, want := range map[string]string{
		"mcp:\n  profiles:\n    safe: browser\n":      "a profile is a list",
		"mcp:\n  profiles:\n    safe: [7]\n":          "a server name is a string",
		"mcp:\n  profiles:\n    safe: [true]\n":       "a server name is a string",
		"mcp:\n  profiles:\n    safe: [[a]]\n":        "a server name is a string",
		"mcp:\n  profiles:\n    safe: [\"\"]\n":       "not a server name",
		"mcp:\n  profiles:\n    safe: [\"a\\nb\"]\n":  "not a server name",
		"mcp:\n  profiles:\n    \"two words\": [a]\n": "not a profile name",
		"mcp:\n  profiles:\n    \"-dash\": [a]\n":     "not a profile name",
		"mcp:\n  profiles: [a]\n":                     "cannot construct",
		"mcp:\n  profile:\n    safe: [a]\n":           "field profile not found",
	} {
		if _, err := Load(write(t, data)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err %v, want %q", data, err, want)
		}
	}
}
