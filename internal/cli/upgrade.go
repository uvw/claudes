package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/upgrade"
	"github.com/uvw/claudes/internal/version"
)

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
		tag, ok := upgrade.Tag(to)
		if !ok {
			return fmt.Errorf("--to takes a release such as v0.2.0, not %q", to)
		}
		to = tag
	}
	exe, err := app.Executable()
	if err != nil {
		return fmt.Errorf("cannot find this claudes binary: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if how := upgrade.ManagedBy(exe); how != "" {
		app.say("This claudes came from %s", how)
		return nil
	}
	if version.Version == "dev" {
		app.say("This claudes was built from source, so upgrade it the same way:")
		app.say("  go install github.com/uvw/claudes/cmd/claudes@latest")
		return nil
	}

	base := upgrade.Base(app.ReleasesURL, to)
	rel, err := upgrade.Find(ctx, app.HTTP, base, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	newer := version.Compare(rel.Version, version.Version)
	switch {
	case to == "" && newer <= 0:
		app.say("claudes %s is the latest release.", version.Version)
		return nil
	case to != "" && newer == 0:
		app.say("claudes %s is already installed.", version.Version)
		return nil
	case check:
		app.say("claudes %s is out; this is %s. Run claudes upgrade to install it.", rel.Version, version.Version)
		return nil
	}

	tmp, err := os.MkdirTemp("", "claudes-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	app.say("Downloading %s", rel.Archive)
	archive, err := upgrade.Download(ctx, app.HTTP, rel, tmp)
	if err != nil {
		return err
	}
	app.say("Checksum matches checksums.txt")
	note, err := app.Provenance(ctx, archive, strings.HasPrefix(base, upgrade.Releases))
	if err != nil {
		return err
	}
	app.say("%s", note)
	next, err := upgrade.Unpack(archive, tmp)
	if err != nil {
		return err
	}
	if err := upgrade.Install(next, exe); err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("%w. Run claudes upgrade as a user who can write %s, or reinstall with the install script", err, filepath.Dir(exe))
		}
		return err
	}
	app.say("Upgraded claudes %s to %s, in %s", version.Version, rel.Version, exe)
	return nil
}
