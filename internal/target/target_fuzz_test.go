package target

import (
	"errors"
	"path"
	"slices"
	"strings"
	"testing"
)

// fuzzRef is a candidate whose label and aliases are free of each other, as
// a Ref may have them.
type fuzzRef struct {
	label   string
	aliases []string
}

func (r fuzzRef) Label() string     { return r.label }
func (r fuzzRef) Aliases() []string { return r.aliases }

func (r fuzzRef) ID() string {
	if len(r.aliases) == 0 {
		return ""
	}
	return r.aliases[0]
}

// fuzzRefs reads candidates split by NULs, each a label and its aliases split
// by \x01.
func fuzzRefs(s string) []fuzzRef {
	var out []fuzzRef
	for c := range strings.SplitSeq(s, "\x00") {
		fields := strings.Split(c, "\x01")
		out = append(out, fuzzRef{label: fields[0], aliases: fields[1:]})
	}
	return out
}

// FuzzTargets checks that no typed target panics Resolve, Match, or Filter,
// that each picks only the candidates its rules name, and that a target
// Resolve finds ambiguous is an error rather than a pick of one.
func FuzzTargets(f *testing.F) {
	var known []string
	for _, s := range sessions {
		known = append(known, s.name+"\x01"+s.id)
	}
	candidates := strings.Join(known, "\x00")
	for _, tokens := range []string{
		"code-reviewer", "242e8b07", "242e", "DB", "trends", "review", "db\x00242e8b07\x00code", "bd4d", "revi",
		"view", "nothing", "*review*", "DB*", "?review", "[abc", "f009", "", " ", "x\x00", "\\", "[]a]", "*",
	} {
		f.Add(tokens, candidates)
	}
	f.Add("2", "agent-2\x0111111111\x00agent-7\x012c4d9e01\x00prod-db-migration\x0133333333\x00frontend\x01db12ab34")
	f.Add("db12", "agent-2\x0111111111\x00frontend\x01db12ab34")
	f.Add("api", "api\x01aaaaaaaa\x00api\x01bbbbbbbb")
	f.Add("api", "API\x00api-2")
	f.Add("K", "K\x00k")
	f.Add("İ", "i̇x\x01İabc")
	f.Add("abcd", "x\x01ABCDEF\x01abcd")
	f.Fuzz(func(t *testing.T, tokens, candidates string) {
		toks, refs := strings.Split(tokens, "\x00"), fuzzRefs(candidates)
		blank := slices.ContainsFunc(toks, func(s string) bool { return strings.TrimSpace(s) == "" })

		got, err := Resolve(toks, refs)
		var want []int
		for _, tok := range toks {
			if step := resolveStep(tok, refs); len(step) == 1 && !slices.Contains(want, step[0]) {
				want = append(want, step[0])
			}
		}
		resolvable := !blank && !slices.ContainsFunc(toks, func(tok string) bool { return len(resolveStep(tok, refs)) != 1 })
		var matchErr *MatchError
		switch {
		case err != nil && !errors.As(err, &matchErr):
			t.Fatalf("Resolve %q: error %T %v is not a MatchError", toks, err, err)
		case err != nil && resolvable:
			t.Fatalf("Resolve %q: %v, though each token names one candidate", toks, err)
		case err == nil && !resolvable:
			t.Fatalf("Resolve %q picked %v, though a token is blank, names none, or is ambiguous", toks, got)
		case err == nil && !slices.EqualFunc(got, pick(refs, want), equalRef):
			t.Fatalf("Resolve %q = %v, want %v", toks, got, pick(refs, want))
		}

		out, missed, err := Filter(toks, refs)
		badPattern := slices.ContainsFunc(toks, func(tok string) bool {
			_, err := path.Match(strings.ToLower(tok), "")
			return isPattern(tok) && err != nil
		})
		if blank || badPattern {
			if err == nil {
				t.Fatalf("Filter %q: no error for a blank token or a bad pattern", toks)
			}
			return
		}
		if err != nil {
			t.Fatalf("Filter %q: %v", toks, err)
		}
		var wantOut []int
		var wantMissed []string
		for i, r := range refs {
			if slices.ContainsFunc(toks, func(tok string) bool { return filterMatches(tok, r) }) {
				wantOut = append(wantOut, i)
			}
		}
		for _, tok := range toks {
			if !slices.ContainsFunc(refs, func(r fuzzRef) bool { return filterMatches(tok, r) }) {
				wantMissed = append(wantMissed, tok)
			}
		}
		if !slices.EqualFunc(out, pick(refs, wantOut), equalRef) || !slices.Equal(missed, wantMissed) {
			t.Fatalf("Filter %q = %v, missed %q; want %v, missed %q", toks, out, missed, pick(refs, wantOut), wantMissed)
		}
		if _, err := Match(toks, refs); (err != nil) != (len(wantMissed) > 0) {
			t.Fatalf("Match %q: error %v, with %q missed", toks, err, wantMissed)
		}
	})
}

// resolveStep is the candidates Resolve's doc comment says tok names: those
// of the first step that has any. Ids are the aliases that are not the label.
func resolveStep(tok string, refs []fuzzRef) []int {
	lower := strings.ToLower(tok)
	steps := []func(r fuzzRef) bool{
		func(r fuzzRef) bool { return slices.Contains(r.aliases, tok) },
		func(r fuzzRef) bool {
			return strings.HasPrefix(strings.ToLower(r.label), lower) || len(lower) >= 4 && slices.ContainsFunc(r.aliases, func(a string) bool {
				return a != r.label && strings.HasPrefix(strings.ToLower(a), lower)
			})
		},
		func(r fuzzRef) bool { return strings.Contains(strings.ToLower(r.label), lower) },
	}
	for _, in := range steps {
		var out []int
		for i, r := range refs {
			if in(r) {
				out = append(out, i)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// filterMatches is whether tok selects r by Match's doc comment.
func filterMatches(tok string, r fuzzRef) bool {
	lower := strings.ToLower(tok)
	if strings.ContainsAny(tok, "*?[") {
		return slices.ContainsFunc(r.aliases, func(a string) bool {
			ok, _ := path.Match(lower, strings.ToLower(a))
			return ok
		})
	}
	return strings.Contains(strings.ToLower(r.label), lower) || len(lower) >= 4 && slices.ContainsFunc(r.aliases, func(a string) bool {
		return a != r.label && strings.HasPrefix(strings.ToLower(a), lower)
	})
}

func pick(refs []fuzzRef, at []int) []fuzzRef {
	out := []fuzzRef{}
	for _, i := range at {
		out = append(out, refs[i])
	}
	return out
}

func equalRef(a, b fuzzRef) bool { return a.label == b.label && slices.Equal(a.aliases, b.aliases) }
