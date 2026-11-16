//go:build unix && !darwin && !linux

package procs

import "errors"

// Executable is not read on this system, so callers fall back to what Claude
// Code's own files say.
func Executable(int) (string, error) {
	return "", errors.New("reading a process's executable is not supported here")
}
