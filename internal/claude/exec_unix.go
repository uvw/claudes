//go:build unix

package claude

import (
	"os/exec"
	"syscall"
)

// Exec replaces this process with the named binary, so its exit code and
// terminal handling are exactly its own. It returns only on failure.
func Exec(binary string, args ...string) error {
	path, err := exec.LookPath(binary)
	if err != nil {
		return err
	}
	return syscall.Exec(path, append([]string{binary}, args...), childEnv())
}
