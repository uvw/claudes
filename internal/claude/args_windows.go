//go:build windows

package claude

import (
	"fmt"
	"path/filepath"
	"strings"
)

// cmdSpecial are the characters cmd.exe reads as syntax even inside the
// quotes Go adds.
const cmdSpecial = "\"%!^&|<>\r\n"

// checkArgs refuses an argument that would not reach claude intact. When the
// claude found on PATH is a batch script, such as npm's claude.cmd shim, Go
// quotes each argument for a program, and cmd.exe then parses the line again:
// a prompt with & in it would run the rest as a command.
func checkArgs(path string, args []string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd", ".bat":
	default:
		return nil
	}
	for _, a := range args {
		if strings.ContainsAny(a, cmdSpecial) {
			return fmt.Errorf("%s is a batch script, which cannot pass %q safely: "+
				"install the native Claude Code, or leave out \" %% ! ^ & | < >", filepath.Base(path), a)
		}
	}
	return nil
}
