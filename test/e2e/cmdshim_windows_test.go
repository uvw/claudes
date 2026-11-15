//go:build windows

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// npm installs claude on Windows as a claude.cmd script, which cmd.exe parses
// again after Go quotes the arguments.
func TestAPromptACmdShimWouldParseIsRefusedBeforeAnythingStops(t *testing.T) {
	w := newWorld(t)
	alpha := w.bg("alpha", "busy")
	shimDir := filepath.Join(w.root, "npm")
	w.writeFile(filepath.Join(shimDir, "claude.cmd"), "@\""+filepath.Join(binDir, "claude.exe")+"\" %*\r\n")
	w.env = append(w.env, "PATH="+shimDir+string(os.PathListSeparator)+binDir)

	r := w.run("restart", "alpha", "--ping-text", `run "the tests & echo INJECTED > injected.txt`)

	if r.code != 1 {
		t.Fatalf("exit %d; stdout %q stderr %q", r.code, r.stdout, r.stderr)
	}
	assertContains(t, r.stderr, "batch script")
	if len(w.actions()) != 0 {
		t.Fatalf("changed sessions: %q", argsOf(w.actions()))
	}
	if _, err := os.Stat(filepath.Join(alpha.Cwd, "injected.txt")); err == nil {
		t.Fatal("cmd.exe ran the injected command")
	}

	w.ok("restart", "alpha", "--ping-text", "carry on with the tests")
	if len(w.actions()) == 0 {
		t.Fatal("a plain prompt did not get through the shim")
	}
}
