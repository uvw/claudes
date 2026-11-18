//go:build unix

package files

import "os"

func rename(from, to string) error { return os.Rename(from, to) }
