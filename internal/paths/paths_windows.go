//go:build windows

package paths

import (
	"os"
	"path/filepath"
)

// configDefault is %APPDATA%\claudes, which roams with the user's profile.
func configDefault() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, app), nil
}

// stateDefault and cacheDefault sit under %LOCALAPPDATA%, which stays on this
// machine.
func stateDefault() (string, error) { return local("state") }
func cacheDefault() (string, error) { return local("cache") }

func local(sub string) (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, app, sub), nil
}

// LegacyState is ~/.local/state/claudes, where claudes kept its state on
// Windows before it used %LOCALAPPDATA%.
func LegacyState() (string, error) { return home(".local", "state", app) }
