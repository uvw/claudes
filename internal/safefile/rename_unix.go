//go:build unix

package safefile

import "os"

func rename(from, to string) error { return os.Rename(from, to) }
