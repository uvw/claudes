package cswap

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// backup copies testdata/sequence.json into a new backup directory.
func backup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("testdata", "sequence.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sequence.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// profile makes a profile directory holding the named files and directories,
// a directory being one that ends in /.
func profile(t *testing.T, dir, name string, holds ...string) {
	t.Helper()
	p := filepath.Join(dir, "sessions", name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, h := range holds {
		var err error
		if d, ok := strings.CutSuffix(h, "/"); ok {
			err = os.MkdirAll(filepath.Join(p, d), 0o755)
		} else {
			err = os.WriteFile(filepath.Join(p, h), []byte("{}"), 0o600)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadSequenceReadsEveryAccountInNumberOrder(t *testing.T) {
	seq, err := ReadSequence(backup(t))
	if err != nil {
		t.Fatal(err)
	}
	if seq.Active != "1" {
		t.Errorf("active %q", seq.Active)
	}
	want := []Account{
		{Number: "1", Email: "you@example.com", UUID: "0b6c1e9a-2f41-4d7a-9c3e-5a8f1d2b7e60", Added: "2026-10-02T18:40:11.004Z", Alias: "Personal"},
		{Number: "2", Email: "you@work.example", UUID: "7d2e4f10-8b3a-4c69-a1f5-3e9c0d6b2a48", OrganizationUUID: "c3a9e1f2-6d4b-4a87-b2c0-9f1e7d5a3b26", OrganizationName: "Example Co", Added: "2026-10-05T08:02:47.930Z"},
		{Number: "3", Email: "you@work.example", UUID: "7d2e4f10-8b3a-4c69-a1f5-3e9c0d6b2a48", OrganizationUUID: "e8f2b4c6-1a3d-4e59-8c7b-2d0f6a9e1c35", OrganizationName: "Other Co", Added: "2026-11-11T12:30:05.118Z", Alias: "side"},
	}
	if d := cmp.Diff(want, seq.Accounts); d != "" {
		t.Fatalf("accounts (-want +got):\n%s", d)
	}
}

func TestReadSequenceSortsNumbersAsNumbers(t *testing.T) {
	dir := t.TempDir()
	data := `{"activeAccountNumber": null, "accounts": {"10": {"email": "a@example.com"}, "9": {"email": "b@example.com"}}}`
	if err := os.WriteFile(filepath.Join(dir, "sequence.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	seq, err := ReadSequence(dir)
	if err != nil || seq.Active != "" || len(seq.Accounts) != 2 || seq.Accounts[0].Number != "9" {
		t.Fatalf("sequence %+v, err %v", seq, err)
	}
}

func TestReadSequenceFailsOnAMissingOrBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadSequence(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
	for _, data := range []string{"[]", `{"accounts": {"x": {}}}`} {
		if err := os.WriteFile(filepath.Join(dir, "sequence.json"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadSequence(dir); err == nil {
			t.Errorf("%s: no error", data)
		}
	}
}

// The slug follows cswap's slugify_email: ASCII letters, digits, '.', '_',
// and '-' stay, and everything else becomes '_'.
func TestSlugSpellsTheEmailAsCswapDoes(t *testing.T) {
	for email, want := range map[string]string{
		"you@example.com":        "you_example.com",
		"first.last+cc@x-y.test": "first.last_cc_x-y.test",
		"zoë@example.com":        "zo__example.com",
		`a\b:c*d?e"f<g>h|i/j`:    "a_b_c_d_e_f_g_h_i_j",
	} {
		if got := Slug(email); got != want {
			t.Errorf("Slug(%q) = %q, want %q", email, got, want)
		}
	}
}

func TestProfilesReadOnlyTheOnesThatMatchAnAccount(t *testing.T) {
	dir := backup(t)
	seq, err := ReadSequence(dir)
	if err != nil {
		t.Fatal(err)
	}
	profile(t, dir, "1-you_example.com", ".claude.json")
	profile(t, dir, "2-you_work.example", "jobs/")
	profile(t, dir, "3-old_example.com", "jobs/")
	profile(t, dir, "4-gone_example.com", ".claude.json")
	profile(t, dir, "3-you_work.example")
	profile(t, dir, "scratch")
	if err := os.WriteFile(filepath.Join(dir, "sessions", ".2-you_work.example.stale"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := Profiles(dir, seq)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range list {
		got[filepath.Base(p.Dir)] = p.Skipped
		if p.Skipped == "" && ProfileName(p.Account) != filepath.Base(p.Dir) {
			t.Errorf("%s belongs to %+v", p.Dir, p.Account)
		}
	}
	want := map[string]string{
		"1-you_example.com":  "",
		"2-you_work.example": "",
		"3-old_example.com":  "account 3 is you@work.example now",
		"3-you_work.example": "it holds neither .claude.json nor jobs",
		"4-gone_example.com": "sequence.json has no account 4",
		"scratch":            "its name does not start with an account number",
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Fatalf("skipped (-want +got):\n%s", d)
	}
	if list[0].Dir != filepath.Join(dir, "sessions", "1-you_example.com") {
		t.Errorf("dir %s", list[0].Dir)
	}
}

func TestProfilesWithoutASessionsDirectoryAreNone(t *testing.T) {
	list, err := Profiles(t.TempDir(), Sequence{})
	if err != nil || list != nil {
		t.Fatalf("list %v, err %v", list, err)
	}
}

// cswap resolves a number first, then an alias in any case, then an email.
func TestLookupFollowsCswapsOrder(t *testing.T) {
	seq, err := ReadSequence(backup(t))
	if err != nil {
		t.Fatal(err)
	}
	numbers := func(id string) []string {
		found, err := seq.Lookup(id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var out []string
		for _, a := range found {
			out = append(out, a.Number)
		}
		return out
	}
	for id, want := range map[string][]string{
		"2":               {"2"},
		"7":               nil,
		"personal":        {"1"},
		"SIDE":            {"3"},
		"you@example.com": {"1"},
		"YOU@example.com": nil,
		"nobody":          nil,
	} {
		if got := numbers(id); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", id, got, want)
		}
	}
	_, err = seq.Lookup("you@work.example")
	if err == nil || !strings.Contains(err.Error(), "accounts 2 (Example Co), 3 (Other Co): name one by its number") {
		t.Fatalf("shared email: %v", err)
	}
}

func TestParseListAndDifferences(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "list.json"))
	if err != nil {
		t.Fatal(err)
	}
	active, listed, err := ParseList(data)
	if err != nil || active != "1" || len(listed) != 3 || listed[2].Alias != "side" {
		t.Fatalf("active %q, listed %+v, err %v", active, listed, err)
	}
	seq, err := ReadSequence(backup(t))
	if err != nil {
		t.Fatal(err)
	}
	if d := Differences(seq, active, listed); d != nil {
		t.Fatalf("the same accounts differ: %q", d)
	}
	listed = append(listed[:1], Listed{Number: 2, Email: "other@example.com"}, Listed{Number: 5, Email: "new@example.com"})
	want := []string{
		"cswap lists account 2 as other@example.com, but sequence.json as you@work.example",
		"cswap lists account 5 (new@example.com), which sequence.json lacks",
		"cswap's active account is 2, but sequence.json says 1",
		"sequence.json holds account 3 (you@work.example), which cswap does not list",
	}
	if d := cmp.Diff(want, Differences(seq, "2", listed)); d != "" {
		t.Fatalf("differences (-want +got):\n%s", d)
	}
}

func TestParseListReportsCswapsError(t *testing.T) {
	_, _, err := ParseList([]byte(`{"schemaVersion":1,"error":{"type":"ConfigError","message":"sequence.json is torn"}}`))
	if err == nil || !strings.Contains(err.Error(), "sequence.json is torn") {
		t.Fatalf("err %v", err)
	}
	if _, _, err := ParseList([]byte("Traceback")); err == nil {
		t.Fatal("no error for text")
	}
}
