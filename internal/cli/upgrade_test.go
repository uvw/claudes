package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// release serves a release of version for this computer, whose claudes binary
// is body, and points h's upgrade at it.
func (h *harness) release(t *testing.T, version string, body []byte) {
	t.Helper()
	var buf bytes.Buffer
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("claudes.exe")
		w.Write(body)
		zw.Close()
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "claudes", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write(body)
		tw.Close()
		gz.Close()
	}
	name := fmt.Sprintf("claudes_%s_%s_%s%s", version, runtime.GOOS, runtime.GOARCH, ext)
	sum := sha256.Sum256(buf.Bytes())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
		case name:
			w.Write(buf.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	h.app.HTTP, h.app.ReleasesURL = srv.Client(), srv.URL
}

// installed puts a claudes binary of version at a path upgrade replaces.
func (h *harness) installed(t *testing.T, version string) string {
	t.Helper()
	saved := Version
	Version = version
	t.Cleanup(func() { Version = saved })
	exe := filepath.Join(h.root, "bin", "claudes")
	writeFile(t, exe, "old binary")
	h.app.Executable = func() (string, error) { return exe, nil }
	h.app.Provenance = func(context.Context, string, bool) (string, error) { return "provenance checked", nil }
	return exe
}

func TestUpgradeInstallsANewerRelease(t *testing.T) {
	h := newHarness(t)
	exe := h.installed(t, "0.1.0")
	h.release(t, "0.2.0", []byte("new binary"))

	h.ok(t, "upgrade", "--check")
	if data, _ := os.ReadFile(exe); string(data) != "old binary" || !strings.Contains(h.out.String(), "0.2.0 is out") {
		t.Fatalf("--check changed the binary or said:\n%s", h.out)
	}

	h.out.Reset()
	h.ok(t, "upgrade")
	if data, _ := os.ReadFile(exe); string(data) != "new binary" {
		t.Fatalf("binary %q", data)
	}
	for _, want := range []string{"Checksum matches", "provenance checked", "Upgraded claudes 0.1.0 to 0.2.0"} {
		if !strings.Contains(h.out.String(), want) {
			t.Fatalf("no %q in:\n%s", want, h.out)
		}
	}
}

func TestUpgradeSaysWhenItIsTheLatest(t *testing.T) {
	h := newHarness(t)
	exe := h.installed(t, "0.2.0")
	h.release(t, "0.2.0", []byte("same"))

	h.ok(t, "upgrade")
	if data, _ := os.ReadFile(exe); string(data) != "old binary" || !strings.Contains(h.out.String(), "is the latest release") {
		t.Fatalf("binary %q:\n%s", data, h.out)
	}
}

func TestUpgradeToAnOlderRelease(t *testing.T) {
	h := newHarness(t)
	exe := h.installed(t, "0.2.0")
	h.release(t, "0.1.0", []byte("older"))

	h.ok(t, "upgrade", "--to", "0.1.0")
	if data, _ := os.ReadFile(exe); string(data) != "older" {
		t.Fatalf("binary %q", data)
	}
	h.fails(t, "upgrade", "--to", "latest")
	if !strings.Contains(h.err.String(), "--to takes a release") {
		t.Fatalf("stderr:\n%s", h.err)
	}
}

func TestUpgradeStopsWhenProvenanceFails(t *testing.T) {
	h := newHarness(t)
	exe := h.installed(t, "0.1.0")
	h.release(t, "0.2.0", []byte("new"))
	h.app.Provenance = func(context.Context, string, bool) (string, error) {
		return "", errors.New("no valid build provenance")
	}

	h.fails(t, "upgrade")
	if data, _ := os.ReadFile(exe); string(data) != "old binary" {
		t.Fatalf("binary %q", data)
	}
}

func TestUpgradeLeavesPackageManagersAndSourceBuildsAlone(t *testing.T) {
	h := newHarness(t)
	h.installed(t, "0.1.0")
	h.release(t, "0.2.0", []byte("new"))
	for exe, want := range map[string]string{
		"/opt/homebrew/Cellar/claudes/0.1.0/bin/claudes":                    "brew upgrade claudes",
		`C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\uvw.claudes\x`: "winget upgrade uvw.claudes",
	} {
		h.app.Executable = func() (string, error) { return exe, nil }
		h.out.Reset()
		h.ok(t, "upgrade")
		if !strings.Contains(h.out.String(), want) {
			t.Fatalf("%s:\n%s", exe, h.out)
		}
	}

	Version = "dev"
	h.app.Executable = func() (string, error) { return filepath.Join(h.root, "bin", "claudes"), nil }
	h.out.Reset()
	h.ok(t, "upgrade")
	if !strings.Contains(h.out.String(), "go install github.com/uvw/claudes/cmd/claudes@latest") {
		t.Fatalf("source build:\n%s", h.out)
	}
}
