package upgrade

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTag(t *testing.T) {
	for in, want := range map[string]string{"0.2.0": "v0.2.0", "v0.2.0": "v0.2.0", "v10.0.12": "v10.0.12"} {
		if got, ok := Tag(in); !ok || got != want {
			t.Errorf("Tag(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "latest", "v0.2", "0.2.0-rc1", "vv0.2.0"} {
		if got, ok := Tag(in); ok {
			t.Errorf("Tag(%q) = %q", in, got)
		}
	}
}

func TestManagedBy(t *testing.T) {
	for exe, want := range map[string]string{
		"/opt/homebrew/Cellar/claudes/0.1.0/bin/claudes":                    "brew upgrade claudes",
		"/home/linuxbrew/.linuxbrew/Cellar/claudes/0.1.0/bin/claudes":       "brew upgrade claudes",
		`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\uvw.claudes\x`: "winget upgrade uvw.claudes",
		"/usr/local/bin/claudes":                                            "",
	} {
		got := ManagedBy(exe)
		if want == "" && got != "" || !strings.Contains(got, want) {
			t.Errorf("ManagedBy(%q) = %q", exe, got)
		}
	}
}

// fakeGH is a which that finds a stand-in gh, which prints its arguments and
// exits with the code codes gives its first argument.
func fakeGH(t *testing.T, codes map[string]int) func(string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in gh is a shell script")
	}
	script := "#!/bin/sh\necho \"gh $*\"\ncase \"$1\" in\n"
	for cmd, code := range codes {
		script += fmt.Sprintf("  %s) exit %d ;;\n", cmd, code)
	}
	script += "esac\n"
	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return func(name string) (string, error) {
		if name != "gh" {
			return "", errors.New("not found")
		}
		return gh, nil
	}
}

func TestProvenance(t *testing.T) {
	ctx := context.Background()
	notFound := func(string) (string, error) { return "", errors.New("not found") }
	if note, err := Provenance(ctx, notFound, "a.tar.gz", false); err != nil || !strings.Contains(note, "did not come from GitHub") {
		t.Errorf("not from GitHub: %q, %v", note, err)
	}
	if note, err := Provenance(ctx, notFound, "a.tar.gz", true); err != nil || !strings.Contains(note, "needs the GitHub CLI") {
		t.Errorf("no gh: %q, %v", note, err)
	}
	signedOut := fakeGH(t, map[string]int{"auth": 1})
	if note, err := Provenance(ctx, signedOut, "a.tar.gz", true); err != nil || !strings.Contains(note, "needs the GitHub CLI") {
		t.Errorf("gh signed out: %q, %v", note, err)
	}
	verified := fakeGH(t, map[string]int{"auth": 0, "attestation": 0})
	if note, err := Provenance(ctx, verified, "a.tar.gz", true); err != nil || note != "Build provenance verified with gh" {
		t.Errorf("verified: %q, %v", note, err)
	}
	rejected := fakeGH(t, map[string]int{"auth": 0, "attestation": 1})
	_, err := Provenance(ctx, rejected, "/tmp/a.tar.gz", true)
	if err == nil || !strings.Contains(err.Error(), "a.tar.gz has no valid build provenance") ||
		!strings.Contains(err.Error(), "gh attestation verify /tmp/a.tar.gz --repo uvw/claudes") {
		t.Errorf("rejected: %v", err)
	}
}
