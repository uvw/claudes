// Package selfupdate finds claudes' release for this computer, downloads and
// checks its archive, and puts the new binary in place of the running one.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Releases is where claudes' releases live.
const Releases = "https://github.com/uvw/claudes/releases"

// The largest checksums.txt and archive claudes downloads.
const (
	maxChecksums = 1 << 20
	maxArchive   = 200 << 20
)

// Release is the archive one release holds for this computer.
type Release struct {
	// Version is the release's version without a leading v, such as 0.2.0.
	Version string
	Archive string
	SHA256  string
	// URL is the folder the archive and checksums.txt download from.
	URL string
}

// Base is the folder of the latest release under releases, or of the release
// tagged tag.
func Base(releases, tag string) string {
	if tag == "" {
		return releases + "/latest/download"
	}
	return releases + "/download/" + tag
}

// Find reads checksums.txt at base and picks the archive for goos and goarch.
func Find(ctx context.Context, client *http.Client, base, goos, goarch string) (Release, error) {
	body, err := get(ctx, client, base+"/checksums.txt", maxChecksums)
	if err != nil {
		return Release{}, err
	}
	defer body.Close()
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	name := regexp.MustCompile(`^claudes_(\d+\.\d+\.\d+)_` + regexp.QuoteMeta(goos+"_"+goarch+ext) + `$`)
	lines := bufio.NewScanner(body)
	for lines.Scan() {
		sum, file, ok := strings.Cut(lines.Text(), "  ")
		if !ok || len(sum) != 64 {
			continue
		}
		if m := name.FindStringSubmatch(file); m != nil {
			return Release{Version: m[1], Archive: file, SHA256: sum, URL: base}, nil
		}
	}
	if err := lines.Err(); err != nil {
		return Release{}, err
	}
	return Release{}, fmt.Errorf("the release at %s has no archive for %s_%s", base, goos, goarch)
}

// Download saves r's archive into dir and checks it against checksums.txt.
func Download(ctx context.Context, client *http.Client, r Release, dir string) (string, error) {
	body, err := get(ctx, client, r.URL+"/"+r.Archive, maxArchive)
	if err != nil {
		return "", err
	}
	defer body.Close()
	path := filepath.Join(dir, r.Archive)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(f, hash), body); err != nil {
		return "", err
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != r.SHA256 {
		return "", fmt.Errorf("%s does not match checksums.txt: the download is damaged or was changed", r.Archive)
	}
	return path, f.Close()
}

// Unpack takes the claudes binary out of archive and writes it into dir.
func Unpack(archive, dir string) (string, error) {
	if strings.HasSuffix(archive, ".zip") {
		return unzip(archive, dir)
	}
	return untar(archive, dir)
}

func untar(archive, dir string) (string, error) {
	f, err := os.Open(archive)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s holds no claudes binary", filepath.Base(archive))
		}
		if err != nil {
			return "", err
		}
		if h.Name == "claudes" && h.Typeflag == tar.TypeReg {
			return write(filepath.Join(dir, "claudes"), io.LimitReader(tr, maxArchive))
		}
	}
}

func unzip(archive, dir string) (string, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "claudes.exe" || !f.Mode().IsRegular() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		return write(filepath.Join(dir, "claudes.exe"), io.LimitReader(rc, maxArchive))
	}
	return "", fmt.Errorf("%s holds no claudes.exe", filepath.Base(archive))
}

func write(path string, r io.Reader) (string, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		return "", errors.Join(err, f.Close())
	}
	return path, f.Close()
}

// Install puts the binary at next in place of exe. The new binary is copied
// beside exe first, so a failure leaves exe as it was.
func Install(next, exe string) error {
	staged := filepath.Join(filepath.Dir(exe), "."+filepath.Base(exe)+".new")
	in, err := os.Open(next)
	if err != nil {
		return err
	}
	defer in.Close()
	if _, err := write(staged, in); err != nil {
		os.Remove(staged)
		return fmt.Errorf("cannot write beside %s: %w", exe, err)
	}
	if err := swap(staged, exe); err != nil {
		os.Remove(staged)
		return err
	}
	return nil
}

func get(ctx context.Context, client *http.Client, url string, limit int64) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("cannot download %s: %s", url, resp.Status)
	}
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(resp.Body, limit), resp.Body}, nil
}
