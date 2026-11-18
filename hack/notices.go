//go:build ignore

// Notices writes docs/third-party-notices.txt: the license of every module
// claudes ships, with its repository.
//
// Usage: go run hack/notices.go [--check]
//
// It runs go-licenses, from the tools module in tools/go.mod, for each platform
// claudes is released for, because some modules, such as golang.org/x/sys, are
// only built on one. It fails when a module's license is not one of the
// allowed ones. With --check, it writes nothing and fails when the committed
// file is out of date.
//
// The file names no module versions, so Dependabot's version bumps, which
// cannot rewrite it, leave it alone unless a license's text changes.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const (
	output    = "docs/third-party-notices.txt"
	ownModule = "github.com/uvw/claudes"
	separator = "\x1e"
	template  = "{{ range . }}{{ .Name }}\n{{ .LicenseName }}\n{{ .LicenseURL }}\n{{ .LicensePath }}\n{{ .LicenseText }}" + separator + "{{ end }}"
)

var (
	platforms = []string{"linux", "darwin", "windows"}
	allowed   = []string{"Apache-2.0", "BSD-2-Clause", "BSD-3-Clause", "ISC", "MIT"}
)

func main() {
	if err := run(slices.Contains(os.Args[1:], "--check")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(check bool) error {
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	found, err := modules(root)
	if err != nil {
		return err
	}
	text := render(found)
	path := filepath.Join(root, output)
	if !check {
		return os.WriteFile(path, []byte(text), 0o644)
	}
	current, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if string(current) != text {
		return fmt.Errorf("%s is out of date: run go run hack/notices.go", output)
	}
	return nil
}

// moduleRoot is the directory above the working directory that holds go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory")
		}
		dir = parent
	}
}

// modules maps each module claudes ships, other than claudes itself, to its
// notice.
func modules(root string) (map[string]string, error) {
	tpl, err := os.CreateTemp("", "notices-*.tpl")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tpl.Name())
	if _, err := tpl.WriteString(template); err != nil {
		tpl.Close()
		return nil, err
	}
	if err := tpl.Close(); err != nil {
		return nil, err
	}
	goLicenses, err := toolPath(root, "go-licenses")
	if err != nil {
		return nil, err
	}
	found := map[string]string{}
	var refused []string
	for _, goos := range platforms {
		cmd := exec.Command(goLicenses, "report", "./...", "--template", tpl.Name())
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+goos)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go-licenses for %s: %w\n%s", goos, err, stderr.String())
		}
		for block := range strings.SplitSeq(string(out), separator) {
			if strings.TrimSpace(block) == "" {
				continue
			}
			parts := strings.SplitN(strings.TrimLeft(block, "\n"), "\n", 5)
			if len(parts) < 5 {
				return nil, fmt.Errorf("go-licenses printed a block claudes cannot read: %q", block)
			}
			name, license, url, path, text := parts[0], parts[1], parts[2], parts[3], parts[4]
			if _, seen := found[name]; name == ownModule || seen {
				continue
			}
			repo, ok := repository(url)
			if !ok {
				return nil, fmt.Errorf("no repository in the license URL of %s: %s", name, url)
			}
			found[name] = fmt.Sprintf("%s\n%s, %s\n\n%s\n", name, license, repo, strings.TrimSpace(text))
			if notice := noticeBeside(path); notice != "" {
				found[name] += "\nNOTICE\n\n" + notice + "\n"
			}
			if !slices.Contains(allowed, license) {
				refused = append(refused, fmt.Sprintf("%s (%s)", name, license))
			}
		}
	}
	if len(refused) > 0 {
		return nil, fmt.Errorf("licenses outside %s: %s", strings.Join(allowed, ", "), strings.Join(refused, ", "))
	}
	return found, nil
}

// repository cuts the version and the file's path from a license URL that
// go-licenses gives, such as https://github.com/spf13/cobra/blob/v1.10.2/LICENSE.txt
// or https://cs.opensource.google/go/x/sys/+/v0.50.0:LICENSE.
func repository(licenseURL string) (string, bool) {
	for _, marker := range []string{"/-/blob/", "/blob/", "/+/"} {
		if repo, _, ok := strings.Cut(licenseURL, marker); ok && strings.HasPrefix(repo, "https://") {
			return repo, true
		}
	}
	return "", false
}

// toolPath builds a tool from tools/go.mod for this machine and returns the
// binary's path. go tool would build it for $GOOS, which may not run here.
func toolPath(root, name string) (string, error) {
	cmd := exec.Command("go", "tool", "-modfile=tools/go.mod", "-n", name)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=", "GOARCH=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("building %s: %w\n%s", name, err, stderr.String())
	}
	return strings.TrimSpace(string(out)), nil
}

// noticeBeside is the NOTICE file next to a module's license, which the
// Apache license requires to travel with the module, or "" when there is none.
func noticeBeside(licensePath string) string {
	for _, name := range []string{"NOTICE", "NOTICE.txt", "NOTICE.md"} {
		if data, err := os.ReadFile(filepath.Join(filepath.Dir(licensePath), name)); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

func render(found map[string]string) string {
	rule := strings.Repeat("=", 79)
	parts := []string{"Third-party software in claudes\n\n" +
		"claudes includes the following modules. Each is listed with its license and\n" +
		"its repository.\n" +
		"Regenerate this file with: go run hack/notices.go\n"}
	for _, name := range slices.Sorted(maps.Keys(found)) {
		parts = append(parts, rule+"\n"+found[name])
	}
	return strings.Join(parts, "\n")
}
