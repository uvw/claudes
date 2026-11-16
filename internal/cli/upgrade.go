package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/selfupdate"
	"github.com/uvw/claudes/internal/version"
)

var releaseTag = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

func (app *App) upgradeCommand() *cobra.Command {
	var check bool
	var to string
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade claudes to its latest release",
		Long: `Download claudes' latest release for this computer, check it against the
release's checksums.txt, and put it in place of this claudes. With the GitHub
CLI signed in, it also checks the archive's build provenance, as the install
scripts do.

A claudes installed with Homebrew, winget, or go install is upgraded the same
way it was installed, so claudes upgrade prints that command instead.

--check only says whether a newer release is out. --to installs the release it
names instead, such as v0.2.0, an older one too.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.upgrade(cmd.Context(), to, check)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "only say whether a newer release is out")
	cmd.Flags().StringVar(&to, "to", "", "install this release instead, such as v0.2.0")
	return cmd
}

func (app *App) upgrade(ctx context.Context, to string, check bool) error {
	if to != "" {
		if !releaseTag.MatchString(to) {
			return fmt.Errorf("--to takes a release such as v0.2.0, not %q", to)
		}
		to = "v" + strings.TrimPrefix(to, "v")
	}
	exe, err := app.Executable()
	if err != nil {
		return fmt.Errorf("cannot find this claudes binary: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if how := managedBy(exe); how != "" {
		app.say("This claudes came from %s", how)
		return nil
	}
	if Version == "dev" {
		app.say("This claudes was built from source, so upgrade it the same way:")
		app.say("  go install github.com/uvw/claudes/cmd/claudes@latest")
		return nil
	}

	base := selfupdate.Base(app.ReleasesURL, to)
	rel, err := selfupdate.Find(ctx, app.HTTP, base, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	newer := version.Compare(rel.Version, Version)
	switch {
	case to == "" && newer <= 0:
		app.say("claudes %s is the latest release.", Version)
		return nil
	case to != "" && newer == 0:
		app.say("claudes %s is already installed.", Version)
		return nil
	case check:
		app.say("claudes %s is out; this is %s. Run claudes upgrade to install it.", rel.Version, Version)
		return nil
	}

	tmp, err := os.MkdirTemp("", "claudes-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	app.say("Downloading %s", rel.Archive)
	archive, err := selfupdate.Download(ctx, app.HTTP, rel, tmp)
	if err != nil {
		return err
	}
	app.say("Checksum matches checksums.txt")
	note, err := app.Provenance(ctx, archive, strings.HasPrefix(base, selfupdate.Releases))
	if err != nil {
		return err
	}
	app.say("%s", note)
	next, err := selfupdate.Unpack(archive, tmp)
	if err != nil {
		return err
	}
	if err := selfupdate.Install(next, exe); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w. Run claudes upgrade as a user who can write %s, or reinstall with the install script", err, filepath.Dir(exe))
		}
		return err
	}
	app.say("Upgraded claudes %s to %s, in %s", Version, rel.Version, exe)
	return nil
}

// managedBy is the package manager that installed exe and the command that
// upgrades it, or "" for a claudes the install script or a download put there.
func managedBy(exe string) string {
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

// ghProvenance checks archive's build provenance with the GitHub CLI, when it
// is installed and signed in. It returns what it found to say; an archive the
// check rejects is an error. fromGitHub is false for a download from anywhere
// else, which has no provenance to check.
func (app *App) ghProvenance(ctx context.Context, archive string, fromGitHub bool) (string, error) {
	if !fromGitHub {
		return "Build provenance not checked: the archive did not come from GitHub", nil
	}
	gh, err := app.Which("gh")
	if err != nil || exec.CommandContext(ctx, gh, "auth", "status").Run() != nil {
		return "Build provenance not checked: it needs the GitHub CLI (gh), signed in", nil
	}
	out, err := exec.CommandContext(ctx, gh, "attestation", "verify", archive, "--repo", "uvw/claudes").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s has no valid build provenance from uvw/claudes: %s", filepath.Base(archive), strings.TrimSpace(string(out)))
	}
	return "Build provenance verified with gh", nil
}
