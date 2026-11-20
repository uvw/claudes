//go:build windows

package claude

import (
	"strings"
	"testing"
)

func TestBatchScriptsRefuseArgumentsCmdWouldParse(t *testing.T) {
	shim := `C:\Users\me\AppData\Roaming\npm\claude.cmd`
	for _, arg := range []string{`run "the tests & echo INJECTED`, "50% done", "a|b", "line\nbreak"} {
		if err := checkArgs(shim, []string{"--bg", "--resume", "sid", arg}); err == nil || !strings.Contains(err.Error(), "batch script") {
			t.Errorf("%q: err %v", arg, err)
		}
	}
	if err := checkArgs(shim, []string{"--bg", "--resume", "16cf5295-0000", "Pick up where you left off."}); err != nil {
		t.Fatalf("plain prompt refused: %v", err)
	}
	if err := checkArgs(`C:\Users\me\.local\bin\claude.exe`, []string{"a & b"}); err != nil {
		t.Fatalf("native binary refused: %v", err)
	}
}
