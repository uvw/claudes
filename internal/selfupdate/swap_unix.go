//go:build unix

package selfupdate

import "os"

// swap renames staged over exe. A running process keeps the old file open, so
// it goes on working.
func swap(staged, exe string) error { return os.Rename(staged, exe) }
