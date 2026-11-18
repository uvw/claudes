package cli

import (
	"encoding/json"
	"testing"
)

func TestDoctorEnvShowsEachColorVariable(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("FORCE_COLOR", "3")
	t.Setenv("CLICOLOR", "0")
	h := newHarness(t)
	h.ok(t, "doctor", "env", "-o", "json")
	var report map[string]map[string]string
	if err := json.Unmarshal(h.out.Bytes(), &report); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	env := report["environment"]
	if env["NO_COLOR"] != `""` || env["CLICOLOR_FORCE"] != "1" || env["FORCE_COLOR"] != "3" || env["CLICOLOR"] != "0" {
		t.Fatalf("environment %v", env)
	}
}
