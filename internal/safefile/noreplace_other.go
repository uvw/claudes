//go:build unix && !darwin && !linux

package safefile

func renameNoReplace(from, to string) error { return renameChecked(from, to) }
