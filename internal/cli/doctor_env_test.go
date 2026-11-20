package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestDoctorEnvShowsEachColorVariable(t *testing.T) {
	h := newHarness(t)
	h.env("NO_COLOR=", "CLICOLOR_FORCE=0", "CLICOLOR_FORCE=1", "FORCE_COLOR=3", "CLICOLOR=0", "PATH=/bin")
	h.ok(t, "doctor", "env", "-o", "json")
	var report map[string]map[string]string
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	env := report["environment"]
	if env["NO_COLOR"] != `""` || env["CLICOLOR_FORCE"] != "1" || env["FORCE_COLOR"] != "3" || env["CLICOLOR"] != "0" {
		t.Fatalf("environment %v", env)
	}
	if env["EDITOR"] != "" || env["PATH"] != "" {
		t.Fatalf("environment %v", env)
	}
	if got, want := report["claudes"]["executable"], filepath.Join(h.root, "bin", "claudes"); got != want {
		t.Fatalf("executable %q, want %q", got, want)
	}
}
