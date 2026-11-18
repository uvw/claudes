package version

import "runtime/debug"

// Version and Commit are claudes' own, set at build time with -ldflags
// "-X github.com/uvw/claudes/internal/version.Version=...".
var (
	Version = "dev"
	Commit  = ""
)

// Build is claudes' own version. It prefers the ldflags values, then the
// module version go install records.
func Build() string {
	if Version != "dev" {
		if Commit != "" {
			return Version + " (" + Commit + ")"
		}
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
