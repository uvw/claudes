package upgrade

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var releaseTag = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// Tag is the tag of the release a user names, such as v0.2.0 for 0.2.0, and
// whether it names one at all.
func Tag(release string) (string, bool) {
	if !releaseTag.MatchString(release) {
		return "", false
	}
	return "v" + strings.TrimPrefix(release, "v"), true
}

// ManagedBy is the package manager that installed exe and the command that
// upgrades it, or "" for a claudes the install script or a download put there.
func ManagedBy(exe string) string {
	// Backslashes too, so a Windows path reads the same on every platform.
	path := strings.ReplaceAll(strings.ToLower(exe), `\`, "/")
	switch {
	// Homebrew links its binaries from the Cellar, on macOS and Linux alike.
	case strings.Contains(path, "/cellar/"):
		return "Homebrew. Upgrade it with:\n  brew upgrade claudes"
	case strings.Contains(path, "/microsoft/winget/"):
		return "winget. Upgrade it with:\n  winget upgrade uvw.claudes"
	}
	return ""
}

// Provenance checks archive's build provenance with the GitHub CLI, when which
// finds it and it is signed in. It returns what it found to say; an archive
// the check rejects is an error. fromGitHub is false for a download from
// anywhere else, which has no provenance to check.
func Provenance(ctx context.Context, which func(string) (string, error), archive string, fromGitHub bool) (string, error) {
	if !fromGitHub {
		return "Build provenance not checked: the archive did not come from GitHub", nil
	}
	gh, err := which("gh")
	signedIn := err == nil && exec.CommandContext(ctx, gh, "auth", "status").Run() == nil
	if !signedIn {
		return "Build provenance not checked: it needs the GitHub CLI (gh), signed in", nil
	}
	out, err := exec.CommandContext(ctx, gh, "attestation", "verify", archive, "--repo", "uvw/claudes").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s has no valid build provenance from uvw/claudes: %s", filepath.Base(archive), strings.TrimSpace(string(out)))
	}
	return "Build provenance verified with gh", nil
}
