// Package version holds claudes' own build version, and compares dotted
// version numbers such as "2.1.325".
package version

import (
	"cmp"
	"strconv"
	"strings"
)

// Compare returns -1, 0, or 1 as a is older than, the same as, or newer than
// b. Parts that are not numbers compare as zero, and a missing part as zero.
func Compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := range max(len(pa), len(pb)) {
		if c := cmp.Compare(at(pa, i), at(pb, i)); c != 0 {
			return c
		}
	}
	return 0
}

// Less reports whether a is an older version than b.
func Less(a, b string) bool { return Compare(a, b) < 0 }

// Minor is the major and minor part of v, such as "2.1" for "2.1.325".
func Minor(v string) string {
	fields := strings.SplitN(trim(v), ".", 3)
	if len(fields) < 2 {
		return trim(v)
	}
	return fields[0] + "." + fields[1]
}

func trim(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }

func parts(v string) []int {
	fields := strings.Split(trim(v), ".")
	out := make([]int, len(fields))
	for i, f := range fields {
		out[i], _ = strconv.Atoi(f)
	}
	return out
}

func at(p []int, i int) int {
	if i < len(p) {
		return p[i]
	}
	return 0
}
