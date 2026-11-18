package style

import (
	"path/filepath"
	"slices"
	"strings"
)

// The ways a table can shorten a directory that does not fit its column.
const (
	DirsMiddle = "middle"
	DirsFish   = "fish"
	DirsFull   = "full"
)

// leadingDirs is how many directories after the root Middle keeps at the start
// of a path, at most.
const leadingDirs = 2

// ShortDir shortens path to at most limit columns, as how says: DirsMiddle,
// DirsFish, or DirsFull. A path that fits is never changed, and DirsFull
// never changes one.
func ShortDir(path string, limit int, how string) string {
	if how == DirsFull || Width(path) <= limit {
		return path
	}
	if how == DirsFish {
		return Fish(path, limit)
	}
	return Middle(path, limit)
}

// Middle shortens path to at most limit columns by putting … in place of the
// directories in its middle. It keeps whole names: the last directory and the
// root, such as ~ or /, then as many directories before the last as fit, then
// up to two after the root. Only a last directory too long to fit on its own
// is cut, in its middle.
func Middle(path string, limit int) string {
	if Width(path) <= limit {
		return path
	}
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	n := len(parts)
	// width is how wide the first head parts, then …, then the last tail
	// parts are, with their separators.
	width := func(head, tail int) int {
		w := head + 1 + tail
		for _, p := range parts[:head] {
			w += Width(p)
		}
		for _, p := range parts[n-tail:] {
			w += Width(p)
		}
		return w
	}
	if n == 1 {
		return cutMiddle(path, limit)
	}
	if n == 2 || width(0, 1) > limit {
		prefix := "…" + sep
		if isRoot(parts[0], path) {
			prefix = parts[0] + sep + strings.Repeat(prefix, min(n-2, 1))
		}
		if Width(prefix) >= limit {
			return cutMiddle(path, limit)
		}
		return prefix + cutMiddle(parts[n-1], limit-Width(prefix))
	}
	head, tail := 0, 1
	fits := func(h, t int) bool { return h+t < n && width(h, t) <= limit }
	if isRoot(parts[0], path) && fits(1, tail) {
		head = 1
	}
	for fits(head, tail+1) {
		tail++
	}
	for most := head + leadingDirs; head < most && fits(head+1, tail); {
		head++
	}
	kept := append(slices.Clip(parts[:head]), "…")
	return strings.Join(append(kept, parts[n-tail:]...), sep)
}

// isRoot reports whether first, the part of path before its first separator,
// is where the path starts from: ~, the empty name before the / of an
// absolute path, or a Windows volume such as C:.
func isRoot(first, path string) bool {
	return first == "~" || first == "" || first == filepath.VolumeName(path)
}

// Fish shortens path to at most limit columns as the fish shell's prompt_pwd
// does: each directory but the last becomes its first character, or its first
// two when it starts with a dot. It shortens them from the left and stops as
// soon as the path fits, so the directories nearest the last stay whole when
// there is room. A path still too long after that is shortened as Middle
// does.
func Fish(path string, limit int) string {
	if Width(path) <= limit {
		return path
	}
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	for i := range parts[:len(parts)-1] {
		parts[i] = initial(parts[i])
		if short := strings.Join(parts, sep); Width(short) <= limit {
			return short
		}
	}
	return Middle(strings.Join(parts, sep), limit)
}

// initial is a directory's name as Fish shortens it.
func initial(name string) string {
	r := []rune(name)
	if len(r) > 1 && r[0] == '.' {
		return string(r[:2])
	}
	if len(r) > 0 {
		return string(r[:1])
	}
	return name
}

// cutMiddle shortens s to at most limit columns by putting … in place of its
// middle.
func cutMiddle(s string, limit int) string {
	if Width(s) <= limit {
		return s
	}
	if limit < 1 {
		return ""
	}
	r := []rune(s)
	// Takes a character from the front, then one from the back, so each end
	// keeps about half.
	front, back, room := 0, len(r), limit-1
	for front < back {
		if w := Width(string(r[front])); w <= room {
			room, front = room-w, front+1
		} else {
			break
		}
		if w := Width(string(r[back-1])); front < back && w <= room {
			room, back = room-w, back-1
		} else {
			break
		}
	}
	return string(r[:front]) + "…" + string(r[back:])
}
