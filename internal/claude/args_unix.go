//go:build unix

package claude

// checkArgs accepts every argument: programs on Unix receive them as given.
func checkArgs(string, []string) error { return nil }
