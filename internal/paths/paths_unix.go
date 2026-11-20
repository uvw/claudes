//go:build unix

package paths

func configDefault() (string, error) { return home(".config", app) }
func stateDefault() (string, error)  { return home(".local", "state", app) }
func cacheDefault() (string, error)  { return home(".cache", app) }

// LegacyState is where an older claudes kept its state on this platform, or ""
// when it is the same as State.
func LegacyState() (string, error) { return "", nil }
