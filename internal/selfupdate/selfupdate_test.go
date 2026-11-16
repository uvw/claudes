package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// archiveOf packs body as the claudes binary the way goreleaser does for goos.
func archiveOf(t *testing.T, goos string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&buf)
		w, err := zw.Create("claudes.exe")
		if err != nil {
			t.Fatal(err)
		}
		w.Write(body)
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "README.md", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg})
	tw.Write([]byte("hi"))
	tw.WriteHeader(&tar.Header{Name: "claudes", Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
	tw.Write(body)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// server serves a release of version with one archive for goos and goarch.
// It returns the server and the archive's name.
func server(t *testing.T, version, goos, goarch string, archive []byte) (*httptest.Server, string) {
	t.Helper()
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	name := fmt.Sprintf("claudes_%s_%s_%s%s", version, goos, goarch, ext)
	sum := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%s  claudes_%s_plan9_amd64.tar.gz\n%s  %s\n%s  install.sh\n",
		strings.Repeat("0", 64), version, hex.EncodeToString(sum[:]), name, strings.Repeat("1", 64))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case "checksums.txt":
			fmt.Fprint(w, checksums)
		case name:
			w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, name
}

func TestFindDownloadUnpackAndInstall(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			srv, name := server(t, "0.2.0", goos, "arm64", archiveOf(t, goos, []byte("new binary")))
			rel, err := Find(context.Background(), srv.Client(), Base(srv.URL, ""), goos, "arm64")
			if err != nil {
				t.Fatal(err)
			}
			if rel.Version != "0.2.0" || rel.Archive != name {
				t.Fatalf("release %+v", rel)
			}
			dir := t.TempDir()
			archive, err := Download(context.Background(), srv.Client(), rel, dir)
			if err != nil {
				t.Fatal(err)
			}
			next, err := Unpack(archive, dir)
			if err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(t.TempDir(), "claudes")
			os.WriteFile(exe, []byte("old binary"), 0o755)
			if err := Install(next, exe); err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(exe); string(data) != "new binary" {
				t.Fatalf("installed %q", data)
			}
			if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
				t.Fatalf("left behind: %v", entries)
			}
		})
	}
}

func TestFindRefusesAReleaseWithoutThisPlatform(t *testing.T) {
	srv, _ := server(t, "0.2.0", "linux", "amd64", []byte("x"))
	if _, err := Find(context.Background(), srv.Client(), Base(srv.URL, "v0.2.0"), "darwin", "arm64"); err == nil ||
		!strings.Contains(err.Error(), "no archive for darwin_arm64") {
		t.Fatalf("err %v", err)
	}
}

func TestDownloadRefusesADamagedArchive(t *testing.T) {
	srv, name := server(t, "0.2.0", "linux", "amd64", []byte("real"))
	rel := Release{Version: "0.2.0", Archive: name, SHA256: strings.Repeat("a", 64), URL: srv.URL + "/latest/download"}
	if _, err := Download(context.Background(), srv.Client(), rel, t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "does not match checksums.txt") {
		t.Fatalf("err %v", err)
	}
}

func TestBase(t *testing.T) {
	if got := Base("https://x/releases", ""); got != "https://x/releases/latest/download" {
		t.Fatal(got)
	}
	if got := Base("https://x/releases", "v0.2.0"); got != "https://x/releases/download/v0.2.0" {
		t.Fatal(got)
	}
}
