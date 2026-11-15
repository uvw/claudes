package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/uvw/claudes/internal/safefile"
)

// cacheFor is how long a cached `claude --version` is trusted. Starting
// claude takes about a second, and the answer only feeds a warning.
const cacheFor = 6 * time.Hour

type cached struct {
	// Binary identifies the claude binary: its real path, time, and size, so
	// an update in place or behind a link is not taken for the same claude.
	Binary    string    `json:"binary"`
	Version   string    `json:"version"`
	CheckedAt time.Time `json:"checkedAt"`
}

// CachePath is the file in dir that caches the claude version.
func CachePath(dir string) string { return filepath.Join(dir, "claude-version.json") }

// CachedVersion returns the Claude Code version that lookup reports, cached
// in dir for six hours per claude binary. An empty dir turns the cache off.
// A cache that cannot be read or written is ignored.
func CachedVersion(dir, binary string, now time.Time, lookup func() (string, error)) (string, error) {
	path := CachePath(dir)
	binary = fingerprint(binary)
	if dir != "" {
		var c cached
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &c) == nil &&
			c.Binary == binary && c.Version != "" && now.Sub(c.CheckedAt) >= 0 && now.Sub(c.CheckedAt) < cacheFor {
			return c.Version, nil
		}
	}
	v, err := lookup()
	if err != nil || dir == "" {
		return v, err
	}
	if data, err := json.Marshal(cached{binary, v, now}); err == nil {
		_ = safefile.WriteFileAll(path, data, 0o644)
	}
	return v, nil
}

// fingerprint is binary's real path with its time and size, or binary itself
// when it cannot be read.
func fingerprint(binary string) string {
	real, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return binary
	}
	info, err := os.Stat(real)
	if err != nil {
		return binary
	}
	return fmt.Sprintf("%s|%d|%d", real, info.ModTime().UnixNano(), info.Size())
}
