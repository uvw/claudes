package transcripts

import (
	"regexp"
	"slices"
	"strings"
)

// Redacted is what Redact puts in place of a secret.
const Redacted = "[redacted]"

// secretRule finds one shape of secret. The secret is the rule's first
// capture group that matched, or the whole match when it has none.
type secretRule struct {
	re *regexp.Regexp
	// hints are what text must contain, in lower case, for the rule to run.
	hints []string
	// ok, when set, rejects a match that only looks like a secret.
	ok func(secret string) bool
}

const (
	// secretName is a name that ends in KEY, TOKEN, SECRET, PASSWORD, or
	// PASSWD after a separator or a lower-case letter, as in api_key,
	// apiKey, GITHUB_TOKEN, and --password, but not monkey.
	secretName = `(?:[A-Za-z0-9_.-]*[_.-])?(?i:key|token|secret|password|passwd)` +
		`|[A-Za-z0-9_.-]*[a-z0-9](?:Key|Token|Secret|Password|Passwd)` +
		`|[A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD)`
	// secretValue is a value of six characters or more, quoted or not. One
	// that starts with $, <, {, or %, such as $GITHUB_TOKEN or <token>,
	// names a secret rather than holding one.
	secretValue = `"([^"\s$<{%][^"\n]{5,})"|'([^'\s$<{%][^'\n]{5,})'` +
		"|([^\\s\"'`$<{%=,;&|()\\[\\]][^\\s\"'`,;&|()\\[\\]]{5,})"
	// nameStart is what comes before a name: the start of the text, or a
	// character a name cannot hold.
	nameStart = `(?:^|[^A-Za-z0-9_.-])`
)

var secretHints = []string{"key", "token", "secret", "passw"}

var secretRules = []secretRule{
	// sk- also covers sk-ant- and sk-proj-. A real key has digits, which
	// names such as sk-learn-something lack.
	{re: regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`), hints: []string{"sk-"}, ok: hasDigit},
	{re: regexp.MustCompile(`\bgh[opusr]_[A-Za-z0-9]{20,}`), hints: []string{"gh"}},
	{re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}`), hints: []string{"github_pat_"}},
	{re: regexp.MustCompile(`\bxox[abposr]-[A-Za-z0-9-]{10,}`), hints: []string{"xox"}},
	{re: regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), hints: []string{"akia", "asia"}},
	{re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}`), hints: []string{"aiza"}},
	{re: regexp.MustCompile(`\bglpat-[0-9A-Za-z_-]{20,}`), hints: []string{"glpat-"}},
	{re: regexp.MustCompile("(?i)\\bauthorization:\\s*(?:bearer|basic|token)\\s+([^\\s\"'`$<{%][^\\s\"'`]*)"),
		hints: []string{"authorization"}},
	// The user and password in a URL, such as https://user:pass@host.
	{re: regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9+.-]*://([^\s/@:]*:[^\s/@]+)@`), hints: []string{"://"}},
	// A private key's body. One cut off before its END line is masked to
	// the end of the text.
	{re: regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----((?s:.+?))(?:-----END |\z)`),
		hints: []string{"private key-----"}},
	// NAME=value, NAME := value, and --name=value.
	{re: regexp.MustCompile(nameStart + `(?:` + secretName + `)\s*:?=\s*(?:` + secretValue + `)`), hints: secretHints},
	// "name": value, 'name': value, a header such as "X-Api-Key: value", and
	// a YAML key at the start of a line.
	{re: regexp.MustCompile(`(?m)(?:["']|^[ \t]*(?:-[ \t]+)?)(?:` + secretName + `)["']?\s*:\s*(?:` + secretValue + `)`),
		hints: secretHints},
	// name: "value", as in a JavaScript object. An unquoted value after a
	// colon is left alone, since prose says "the key: ...".
	{re: regexp.MustCompile(nameStart + `(?:` + secretName + `)\s*:\s*(?:"([^"\s$<{%][^"\n]{5,})"|'([^'\s$<{%][^'\n]{5,})')`),
		hints: secretHints},
	// --password value.
	{re: regexp.MustCompile(`(?:^|\s)--(?:[A-Za-z0-9-]*-)?(?i:key|token|secret|password|passwd)\s+(?:` + secretValue + `)`),
		hints: secretHints},
}

func hasDigit(s string) bool { return strings.ContainsAny(s, "0123456789") }

// span is text[start:end], a secret to mask.
type span struct{ start, end int }

// secrets are the spans of text that hold secrets, in order, merged where
// they overlap.
func secrets(text string) []span {
	lower := strings.ToLower(text)
	var found []span
	for _, r := range secretRules {
		if !slices.ContainsFunc(r.hints, func(h string) bool { return strings.Contains(lower, h) }) {
			continue
		}
		for _, m := range r.re.FindAllStringSubmatchIndex(text, -1) {
			s := span{m[0], m[1]}
			for g := 2; g < len(m); g += 2 {
				if m[g] >= 0 {
					s = span{m[g], m[g+1]}
					break
				}
			}
			if s.start < s.end && (r.ok == nil || r.ok(text[s.start:s.end])) {
				found = append(found, s)
			}
		}
	}
	slices.SortFunc(found, func(a, b span) int { return a.start - b.start })
	var out []span
	for _, s := range found {
		if n := len(out); n > 0 && s.start <= out[n-1].end {
			out[n-1].end = max(out[n-1].end, s.end)
			continue
		}
		out = append(out, s)
	}
	return out
}

// Redact replaces each secret in text with Redacted: provider keys such as
// sk-… and ghp_…, Authorization credentials, the values of names that end
// in KEY, TOKEN, SECRET, PASSWORD, or PASSWD, passwords in URLs, and private
// key blocks. A name stays, so GITHUB_TOKEN=abc… becomes
// GITHUB_TOKEN=[redacted].
func Redact(text string) string {
	out, _, _ := redactAround(text, 0, 0)
	return out
}

// redactAround redacts text and moves the match text[start:end] to where it
// lands in the result. A match that starts or ends inside a secret covers
// its marker.
func redactAround(text string, start, end int) (string, int, int) {
	found := secrets(text)
	if len(found) == 0 {
		return text, start, end
	}
	var b strings.Builder
	last := 0
	for _, s := range found {
		b.WriteString(text[last:s.start])
		b.WriteString(Redacted)
		last = s.end
	}
	b.WriteString(text[last:])
	return b.String(), moved(found, start, false), moved(found, end, true)
}

// moved is where offset at lands once the secrets found are masked.
func moved(found []span, at int, isEnd bool) int {
	shift := 0
	for _, s := range found {
		switch {
		case at >= s.end:
			shift += len(Redacted) - (s.end - s.start)
		case at > s.start && isEnd:
			return s.start + shift + len(Redacted)
		case at > s.start:
			return s.start + shift
		default:
			return at + shift
		}
	}
	return at + shift
}
