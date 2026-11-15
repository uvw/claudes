// Package target turns the names and ids a user types into sessions.
package target

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// Ref is anything a target can select.
type Ref interface {
	// Label is the name shown to the user and searched by part of a name.
	Label() string
	// ID is the id shown beside the label when a target is ambiguous.
	ID() string
	// Aliases are the exact strings that select it: names and ids.
	Aliases() []string
}

// Resolve returns the candidates the tokens name, in token order and without
// duplicates. A token matches an exact name or id first, then the start of a
// name or id, then a part of a name, ignoring case from the second step on. The
// first step with any match decides. A token that matches nothing, or more than
// one candidate in that step, is an error that lists the candidates.
func Resolve[T Ref](tokens []string, candidates []T) ([]T, error) {
	var out []T
	seen := map[int]bool{}
	if err := noEmpty(tokens); err != nil {
		return nil, err
	}
	for _, token := range tokens {
		i, err := match(token, candidates)
		if err != nil {
			return nil, err
		}
		if !seen[i] {
			seen[i] = true
			out = append(out, candidates[i])
		}
	}
	return out, nil
}

// Match returns every candidate the tokens match, in candidate order and
// without duplicates, for commands that only look. A token with *, ?, or [ is
// a pattern over whole names and ids. Any other token matches part of a name,
// or the start of an id when it has at least four characters. Both ignore
// case. A token that matches nothing is an error that lists the candidates.
func Match[T Ref](tokens []string, candidates []T) ([]T, error) {
	out, missed, err := Filter(tokens, candidates)
	if err == nil && len(missed) > 0 {
		return nil, &MatchError{msg: fmt.Sprintf("no session matches %q. Sessions: %s", missed[0], describe(candidates, nil)), NoMatch: true}
	}
	return out, err
}

// Filter matches tokens as Match does, but a token that matches nothing is
// returned in missed rather than being an error, for listings. out is never
// nil.
func Filter[T Ref](tokens []string, candidates []T) (out []T, missed []string, err error) {
	if err := noEmpty(tokens); err != nil {
		return nil, nil, err
	}
	hit := make([]bool, len(candidates))
	for _, token := range tokens {
		var matches []int
		if isPattern(token) {
			if matches, err = matchPattern(token, candidates); err != nil {
				return nil, nil, err
			}
		} else {
			matches = matchWord(token, candidates)
		}
		if len(matches) == 0 {
			missed = append(missed, token)
		}
		for _, i := range matches {
			hit[i] = true
		}
	}
	out = []T{}
	for i, c := range candidates {
		if hit[i] {
			out = append(out, c)
		}
	}
	return out, missed, nil
}

// isPattern reports whether a token is a pattern rather than a name or id.
func isPattern(token string) bool { return strings.ContainsAny(token, "*?[") }

func matchPattern[T Ref](pattern string, candidates []T) ([]int, error) {
	pattern = strings.ToLower(pattern)
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, fmt.Errorf("%q is a bad pattern: %w", pattern, err)
	}
	return find(candidates, func(c T) bool {
		return slices.ContainsFunc(c.Aliases(), func(a string) bool {
			ok, _ := path.Match(pattern, strings.ToLower(a))
			return ok
		})
	}), nil
}

func matchWord[T Ref](word string, candidates []T) []int {
	needle := strings.ToLower(word)
	return find(candidates, func(c T) bool {
		return strings.Contains(strings.ToLower(c.Label()), needle) || startsID(c, needle)
	})
}

func match[T Ref](token string, candidates []T) (int, error) {
	matches := firstStep(token, candidates)
	if len(matches) == 0 {
		return 0, &MatchError{msg: fmt.Sprintf("no session matches %q. Sessions: %s", token, describe(candidates, nil))}
	}
	return one(token, candidates, matches)
}

// firstStep returns the matches of the first step that has any: exact names
// and ids, then the start of a name or id, then part of a name.
func firstStep[T Ref](token string, candidates []T) []int {
	exact := find(candidates, func(c T) bool { return slices.Contains(c.Aliases(), token) })
	if len(exact) > 0 {
		return exact
	}
	needle := strings.ToLower(token)
	prefix := find(candidates, func(c T) bool { return startsName(c, needle) || startsID(c, needle) })
	if len(prefix) > 0 {
		return prefix
	}
	return find(candidates, func(c T) bool { return strings.Contains(strings.ToLower(c.Label()), needle) })
}

// minIDPrefix is the shortest start of an id that matches. Ids are random hex,
// so a shorter word, such as "2" or "db", would match one by accident.
const minIDPrefix = 4

func startsName[T Ref](c T, needle string) bool {
	return strings.HasPrefix(strings.ToLower(c.Label()), needle)
}

func startsID[T Ref](c T, needle string) bool {
	if len(needle) < minIDPrefix {
		return false
	}
	for _, a := range c.Aliases() {
		if a != c.Label() && strings.HasPrefix(strings.ToLower(a), needle) {
			return true
		}
	}
	return false
}

func one[T Ref](token string, candidates []T, matches []int) (int, error) {
	if len(matches) == 1 {
		return matches[0], nil
	}
	return 0, &MatchError{msg: fmt.Sprintf("%q matches %d sessions: %s. Use a longer name or an id",
		token, len(matches), describe(candidates, matches))}
}

// MatchError is a target that names no session, or several where one is
// needed.
type MatchError struct {
	msg string
	// NoMatch is set when Match found nothing for a target, which a listing
	// can show as an empty list.
	NoMatch bool
}

func (e *MatchError) Error() string { return e.msg }

func find[T Ref](candidates []T, ok func(T) bool) []int {
	var out []int
	for i, c := range candidates {
		if ok(c) {
			out = append(out, i)
		}
	}
	return out
}

func describe[T Ref](candidates []T, only []int) string {
	if only == nil {
		for i := range candidates {
			only = append(only, i)
		}
	}
	if len(only) == 0 {
		return "none"
	}
	more := 0
	if len(only) > describeLimit {
		more, only = len(only)-describeLimit, only[:describeLimit]
	}
	names := make([]string, 0, len(only)+1)
	for _, i := range only {
		c := candidates[i]
		if id := c.ID(); id != "" && id != c.Label() {
			names = append(names, fmt.Sprintf("%s (%s)", c.Label(), id))
		} else {
			names = append(names, c.Label())
		}
	}
	if more > 0 {
		names = append(names, fmt.Sprintf("and %d more", more))
	}
	return strings.Join(names, ", ")
}

// noEmpty refuses an empty target, such as an unset shell variable, which
// would otherwise match a session with no name.
func noEmpty(tokens []string) error {
	for _, t := range tokens {
		if strings.TrimSpace(t) == "" {
			return &MatchError{msg: "an empty target names no session"}
		}
	}
	return nil
}

// describeLimit caps the sessions an error lists, so a miss among a long
// history of past sessions stays readable.
const describeLimit = 10
