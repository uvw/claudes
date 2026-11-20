//go:build unix

package shell

import "os/exec"

// posixShells are the installed shells a printed command may be pasted into,
// each with the flags that run a script without reading any startup file.
func posixShells() [][]string {
	var out [][]string
	for _, sh := range [][]string{{"sh", "-c"}, {"bash", "--norc", "--noprofile", "-c"}, {"zsh", "-f", "-c"}} {
		if path, err := exec.LookPath(sh[0]); err == nil {
			out = append(out, append([]string{path}, sh[1:]...))
		}
	}
	return out
}
