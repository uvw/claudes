package accounts

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/uvw/claudes/internal/claude"
)

const sequence = `{
  "activeAccountNumber": 1,
  "accounts": {
    "1": {"email": "you@example.com", "alias": "personal"},
    "2": {"email": "you@work.example", "organizationName": "Example Co"},
    "3": {"email": "you@work.example", "organizationName": "Other Co", "alias": "side"},
    "4": {"email": "lab@example.com"}
  }
}`

// layout makes a home directory with ~/.claude, and a cswap backup directory
// with sequence.json and profiles for accounts 1, 2, and 3, and a stale one.
func layout(t *testing.T) (home, backup string) {
	t.Helper()
	root := t.TempDir()
	home, backup = filepath.Join(root, "home"), filepath.Join(root, "backup")
	mkdir(t, filepath.Join(home, ".claude", "jobs"))
	mkdir(t, filepath.Join(backup, "sessions", "1-you_example.com", "jobs"))
	mkdir(t, filepath.Join(backup, "sessions", "2-you_work.example", "jobs"))
	mkdir(t, filepath.Join(backup, "sessions", "3-you_work.example", "jobs"))
	mkdir(t, filepath.Join(backup, "sessions", "5-gone_example.com", "jobs"))
	if err := os.WriteFile(filepath.Join(backup, "sequence.json"), []byte(sequence), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, backup
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func profile(backup, name string) string { return filepath.Join(backup, "sessions", name) }

func TestWithoutCswapOrConfigThereIsOneAccount(t *testing.T) {
	home, _ := layout(t)
	f := Find(Options{Home: home})
	want := []Account{{Name: "~/.claude", Dir: filepath.Join(home, ".claude"), ConfigFile: filepath.Join(home, ".claude.json"), From: []string{FromHome}}}
	if d := cmp.Diff(want, f.Accounts, cmp.AllowUnexported(Account{})); d != "" {
		t.Fatalf("accounts (-want +got):\n%s", d)
	}
	env := filepath.Join(home, "other")
	f = Find(Options{Home: home, Env: env})
	if len(f.Accounts) != 1 || f.Accounts[0].Dir != env || f.Accounts[0].ConfigFile != filepath.Join(env, ".claude.json") || !f.Accounts[0].ConfigDir.Own() {
		t.Fatalf("accounts %+v", f.Accounts)
	}
}

func TestCswapProfilesAreAccounts(t *testing.T) {
	home, backup := layout(t)
	f := Find(Options{Home: home, CswapDir: backup})
	if len(f.Problems) != 0 {
		t.Fatal(f.Problems)
	}
	type row struct {
		Name, Dir string
		Config    claude.ConfigDir
		From      string
		Number    string
	}
	var got []row
	for _, a := range f.Accounts {
		got = append(got, row{a.Name, a.Dir, a.ConfigDir, strings.Join(a.From, "+"), a.Cswap.Number})
	}
	want := []row{
		{"personal", filepath.Join(home, ".claude"), claude.ConfigDir{}, "~/.claude", "1"},
		{"personal", profile(backup, "1-you_example.com"), claude.ConfigDir{Path: profile(backup, "1-you_example.com")}, "cswap", "1"},
		{"you@work.example", profile(backup, "2-you_work.example"), claude.ConfigDir{Path: profile(backup, "2-you_work.example")}, "cswap", "2"},
		{"side", profile(backup, "3-you_work.example"), claude.ConfigDir{Path: profile(backup, "3-you_work.example")}, "cswap", "3"},
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Fatalf("accounts (-want +got):\n%s", d)
	}
	if len(f.Skipped) != 1 || filepath.Base(f.Skipped[0].Dir) != "5-gone_example.com" {
		t.Fatalf("skipped %+v", f.Skipped)
	}
}

// Inside a session cswap run started, claudes runs as the profile, and
// reaches ~/.claude with CLAUDE_CONFIG_DIR unset.
func TestRunningInAProfileKeepsItsOwnConfigDir(t *testing.T) {
	home, backup := layout(t)
	work := profile(backup, "2-you_work.example")
	f := Find(Options{Home: home, Env: work, CswapDir: backup})
	if len(f.Accounts) != 4 {
		t.Fatalf("accounts %+v", f.Accounts)
	}
	own, dot := f.Accounts[0], f.Accounts[1]
	if own.Dir != work || !own.ConfigDir.Own() || own.Name != "you@work.example" || !slices.Equal(own.From, []string{FromEnv, FromCswap}) {
		t.Errorf("own %+v", own)
	}
	if dot.Dir != filepath.Join(home, ".claude") || dot.ConfigDir != (claude.ConfigDir{Unset: true}) || dot.ConfigFile != filepath.Join(home, ".claude.json") {
		t.Errorf("~/.claude %+v", dot)
	}
}

func TestCswapWithoutASequenceAddsNothing(t *testing.T) {
	home, _ := layout(t)
	f := Find(Options{Home: home, CswapDir: t.TempDir()})
	if len(f.Accounts) != 1 || f.Cswap || len(f.Problems) != 0 {
		t.Fatalf("found %+v", f)
	}
}

func TestABrokenSequenceIsAProblemNotAFailure(t *testing.T) {
	home, backup := layout(t)
	if err := os.WriteFile(filepath.Join(backup, "sequence.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := Find(Options{Home: home, CswapDir: backup})
	if len(f.Accounts) != 1 || len(f.Problems) != 1 {
		t.Fatalf("found %+v", f)
	}
}

func TestConfigNamesAndAddsDirectories(t *testing.T) {
	home, backup := layout(t)
	lab := filepath.Join(home, "lab")
	mkdir(t, lab)
	f := Find(Options{Home: home, CswapDir: backup, Dirs: map[string]string{
		"work":    profile(backup, "2-you_work.example"),
		"lab":     lab,
		"missing": filepath.Join(home, "missing"),
	}})
	if len(f.Problems) != 1 || !strings.Contains(f.Problems[0].Error(), "accounts.dirs.missing") {
		t.Errorf("problems %v", f.Problems)
	}
	names := []string{}
	for _, a := range f.Accounts {
		names = append(names, a.Name)
	}
	if want := []string{"personal", "personal", "work", "side", "lab"}; !slices.Equal(names, want) {
		t.Fatalf("names %v, want %v", names, want)
	}
	if a := f.Accounts[4]; a.ConfigDir != (claude.ConfigDir{Path: lab}) || !slices.Equal(a.From, []string{FromConfig}) {
		t.Errorf("lab %+v", a)
	}
	if a := f.Accounts[2]; !slices.Equal(a.From, []string{FromCswap, FromConfig}) {
		t.Errorf("work %+v", a)
	}
}

func TestConfigCanNameHomeWhenClaudesRunsElsewhere(t *testing.T) {
	home, _ := layout(t)
	f := Find(Options{Home: home, Env: filepath.Join(home, "elsewhere"), Dirs: map[string]string{"main": filepath.Join(home, ".claude")}})
	if len(f.Accounts) != 2 {
		t.Fatalf("accounts %+v", f.Accounts)
	}
	if a := f.Accounts[1]; a.Name != "main" || a.ConfigDir != (claude.ConfigDir{Unset: true}) || a.ConfigFile != filepath.Join(home, ".claude.json") {
		t.Fatalf("main %+v", a)
	}
}

func TestResolveFollowsCswapsOrderAfterConfigNames(t *testing.T) {
	home, backup := layout(t)
	f := Find(Options{Home: home, CswapDir: backup, Dirs: map[string]string{"work": profile(backup, "2-you_work.example")}})
	dirs := func(token string) []string {
		t.Helper()
		found, err := f.Resolve(token)
		if err != nil {
			t.Fatalf("%s: %v", token, err)
		}
		var out []string
		for _, a := range found {
			out = append(out, filepath.Base(a.Dir))
		}
		return out
	}
	for token, want := range map[string][]string{
		"work":                                {"2-you_work.example"},
		"1":                                   {".claude", "1-you_example.com"},
		"PERSONAL":                            {".claude", "1-you_example.com"},
		"you@example.com":                     {".claude", "1-you_example.com"},
		"3":                                   {"3-you_work.example"},
		"side":                                {"3-you_work.example"},
		profile(backup, "3-you_work.example"): {"3-you_work.example"},
	} {
		if got := dirs(token); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", token, got, want)
		}
	}
	for token, want := range map[string]string{
		"you@work.example": "name one by its number",
		"4":                "cswap account 4 (lab@example.com) has no profile claudes reads",
		"nobody":           `no account "nobody". Accounts: personal, work, side`,
	} {
		if _, err := f.Resolve(token); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want %q", token, err, want)
		}
	}
}

func TestResolveWithoutCswapTakesANameOrDirectory(t *testing.T) {
	home, _ := layout(t)
	f := Find(Options{Home: home})
	for _, token := range []string{"~/.claude", filepath.Join(home, ".claude")} {
		if found, err := f.Resolve(token); err != nil || len(found) != 1 {
			t.Errorf("%s: %+v, %v", token, found, err)
		}
	}
	if _, err := f.Resolve("1"); err == nil {
		t.Error("a number named an account without cswap")
	}
}

func TestConfigDirOfADirectoryNoLongerAnAccount(t *testing.T) {
	home, backup := layout(t)
	work := profile(backup, "2-you_work.example")
	f := Find(Options{Home: home, Env: work, CswapDir: backup})
	for dir, want := range map[string]claude.ConfigDir{
		work:                                 {},
		profile(backup, "1-you_example.com"): {Path: profile(backup, "1-you_example.com")},
		filepath.Join(home, ".claude"):       {Unset: true},
		filepath.Join(home, "gone"):          {Path: filepath.Join(home, "gone")},
	} {
		if got := f.ConfigDirOf(dir); got != want {
			t.Errorf("%s: %+v, want %+v", dir, got, want)
		}
	}
	f = Find(Options{Home: home, Env: work})
	if got := f.ConfigDirOf(filepath.Join(home, ".claude")); got != (claude.ConfigDir{Unset: true}) {
		t.Errorf("~/.claude without cswap: %+v", got)
	}
}
