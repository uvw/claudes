package mod

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

const sid = "0b7c3f9e-1111-2222-3333-444455556666"

func channel(t *testing.T) Channel {
	t.Helper()
	return Channel{Dir: filepath.Join(t.TempDir(), "state", "mod")}
}

func TestSendWritesARequestOnlyTheUserCanRead(t *testing.T) {
	c := channel(t)
	reqs := []Request{NewRequest("mcp", "disable browser"), NewRequest("mcp", "enable web")}
	expires := time.UnixMilli(1_700_000_000_000)
	if err := c.Send(sid, reqs, expires); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.Dir, "requests", sid+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got requestFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Protocol != Protocol || got.Session != sid || got.Expires != expires.UnixMilli() || len(got.Requests) != 2 {
		t.Fatalf("got %+v", got)
	}
	if runtime.GOOS == "windows" {
		return
	}
	for _, p := range []string{c.Dir, filepath.Join(c.Dir, "requests"), filepath.Join(c.Dir, "results")} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", p, info.Mode(), err)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("request: %v %v", info.Mode(), err)
	}
}

func TestPrepareTightensADirectoryTheModCreated(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no mode bits for others")
	}
	c := channel(t)
	if err := os.MkdirAll(filepath.Join(c.Dir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := c.Prepare(); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(c.Dir); info.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", info.Mode())
	}
}

func TestRequestIDsAreFreshHex(t *testing.T) {
	a, b := NewRequest("mcp", "x"), NewRequest("mcp", "x")
	if a.ID == b.ID || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(a.ID) {
		t.Fatalf("%q %q", a.ID, b.ID)
	}
}

func TestSendRefusesWhatIsNotASessionID(t *testing.T) {
	c := channel(t)
	for _, bad := range []string{"../../x", "", "abc"} {
		if err := c.Send(bad, nil, time.Now()); err == nil {
			t.Fatalf("sent to %q", bad)
		}
		if _, _, err := c.Presence(bad); err == nil {
			t.Fatalf("read presence of %q", bad)
		}
		if err := c.Clear(bad, nil); err == nil {
			t.Fatalf("cleared %q", bad)
		}
	}
}

func TestResultsReadWhatTheModWroteAndClearRemovesIt(t *testing.T) {
	c := channel(t)
	done, later, torn := NewRequest("mcp", "disable a"), NewRequest("mcp", "disable b"), NewRequest("mcp", "disable c")
	reqs := []Request{done, later, torn}
	if err := c.Send(sid, reqs, time.Now()); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(c.Dir, "results", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(done.ID+".json", `{"id":"`+done.ID+`","session":"`+sid+`","status":"done","at":5,"version":"0.1.0"}`)
	write(torn.ID+".json", `{"id":"`)

	got, err := c.Results(reqs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[done.ID].Status != Done || got[done.ID].Version != "0.1.0" {
		t.Fatalf("got %+v", got)
	}

	if err := c.Clear(sid, reqs); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(c.Dir, "requests", sid+".json"), filepath.Join(c.Dir, "results", done.ID+".json")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s is still there: %v", p, err)
		}
	}
}

func TestPresence(t *testing.T) {
	c := channel(t)
	if _, ok, err := c.Presence(sid); ok || err != nil {
		t.Fatalf("found presence in an empty channel: %v", err)
	}
	if err := c.Prepare(); err != nil {
		t.Fatal(err)
	}
	text := `{"protocol":1,"session":"` + sid + `","version":"0.1.0","at":1700000000000}`
	if err := os.WriteFile(filepath.Join(c.Dir, "sessions", sid+".json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	p, ok, err := c.Presence(sid)
	if err != nil || !ok || p.Version != "0.1.0" || !p.Time().Equal(time.UnixMilli(1_700_000_000_000)) {
		t.Fatalf("got %+v %v %v", p, ok, err)
	}
	if err := c.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Dir); !os.IsNotExist(err) {
		t.Fatalf("channel still there: %v", err)
	}
}

func TestDefaultChannelIsUnderTheStateDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	c, err := DefaultChannel()
	if err != nil || c.Dir != filepath.Join(dir, "claudes", "mod") {
		t.Fatalf("got %q, %v", c.Dir, err)
	}
}
