package compat

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFlagOn(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		path := filepath.Join(dir, ".claude.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
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
		on, found, err := FlagOn(write(tc.body), PinsFlag)
		if err != nil || on != tc.on || found != tc.found {
			t.Errorf("%s: on %v, found %v, err %v", tc.body, on, found, err)
		}
	}
	if _, found, err := FlagOn(filepath.Join(dir, "missing.json"), PinsFlag); found || err != nil {
		t.Errorf("a missing file: found %v, err %v", found, err)
	}
	if _, _, err := FlagOn(write("not json"), PinsFlag); err == nil {
		t.Error("a file that is not JSON gave no error")
	}
}
