package transcripts

import (
	"strings"
	"testing"
)

// The made-up secrets are put together when the tests run, so a secret
// scanner reading this file finds none.
const fakeChars = "Zq8xW3vN5mK7pL2rT9yB4cF6hJ1dS0gA"

// fakeKey is prefix followed by n made-up characters.
func fakeKey(prefix string, n int) string {
	return prefix + strings.Repeat(fakeChars, n/len(fakeChars)+1)[:n]
}

func TestRedactMasksSecrets(t *testing.T) {
	val := fakeKey("", 24)
	privateKey := "PRIVATE" + " KEY-----"
	rsaBegin, rsaEnd, sshBegin := "-----BEGIN RSA "+privateKey, "-----END RSA "+privateKey, "-----BEGIN OPENSSH "+privateKey
	for _, tc := range []struct{ in, want string }{
		{fakeKey("sk-ant-api03-", 40), "[redacted]"},
		{"use " + fakeKey("sk-proj-", 30) + " here", "use [redacted] here"},
		{"key is " + fakeKey("gh"+"p_", 36) + " ok", "key is [redacted] ok"},
		{fakeKey("gh"+"o_", 36), "[redacted]"},
		{fakeKey("github"+"_pat_", 60), "[redacted]"},
		{"slack " + fakeKey("xo"+"xb-", 30), "slack [redacted]"},
		{"id AK" + "IAQWERTYUIOP123456 ok", "id [redacted] ok"},
		{fakeKey("AI"+"za", 35), "[redacted]"},
		{fakeKey("gl"+"pat-", 20), "[redacted]"},
		{`curl -H "Authorization: Bearer ` + val + `" https://api.example.com`,
			`curl -H "Authorization: Bearer [redacted]" https://api.example.com`},
		{"authorization: basic " + val, "authorization: basic [redacted]"},
		{"export GITHUB_TOKEN=" + val + "; make", "export GITHUB_TOKEN=[redacted]; make"},
		{"tool --password=" + val, "tool --password=[redacted]"},
		{"mysql --password " + val + " db", "mysql --password [redacted] db"},
		{`{"api_key": "` + val + `"}`, `{"api_key": "[redacted]"}`},
		{`{ apiKey: "` + val + `" }`, `{ apiKey: "[redacted]" }`},
		{"db:\n  password: " + val + "\n", "db:\n  password: [redacted]\n"},
		{`-H "X-Api-Key: ` + val + `"`, `-H "X-Api-Key: [redacted]"`},
		{"secret := '" + val + "'", "secret := '[redacted]'"},
		{"GET /x?access_token=" + val + "&page=2", "GET /x?access_token=[redacted]&page=2"},
		{"A_KEY=" + val + " B_SECRET=" + val, "A_KEY=[redacted] B_SECRET=[redacted]"},
		{"git clone https://deploy:" + val + "@example.com/repo.git", "git clone https://[redacted]@example.com/repo.git"},
		{"GITHUB_TOKEN=" + fakeKey("gh"+"p_", 36), "GITHUB_TOKEN=[redacted]"},
		{rsaBegin + "\n" + val + "\n" + rsaEnd + "\nnext", rsaBegin + "[redacted]" + rsaEnd + "\nnext"},
		{sshBegin + "\n" + val, sshBegin + "[redacted]"},
	} {
		if got := Redact(tc.in); got != tc.want {
			t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRedactLeavesTheRestAlone(t *testing.T) {
	for _, in := range []string{
		"please deploy the zebra service today",
		"commit 3f2a9c1e8b7d6a5f4e3d2c1b0a9f8e7d6c5b4a3f\nAuthor: Someone",
		"a1b2c3d fix the build (#12)",
		"session 4f9c2e1a-8b3d-4c7e-9a2f-1d6b8e3c5a70 resumed",
		"key=abc TOKEN=1234 password: x",
		"the primary key: an id column, and the token: a string",
		"monkey=bananas123 turkey=sandwich",
		"pip install sk-learn-is-not-a-package-name",
		"TOKEN=$GITHUB_TOKEN gh api /user",
		`curl -H "Authorization: Bearer $TOKEN" https://api.example.com`,
		"password: ${{ secrets.PASSWORD }}",
		"api_key: <your key here>",
		"https://example.com/path?q=1 ssh://git@example.com/repo.git http://localhost:8080/x",
		"-----BEGIN PUBLIC KEY-----\nabc\n-----END PUBLIC KEY-----",
		"the keyboard tokens passwords secrets",
		"AKIA is a prefix",
		"if password == \"\" {",
	} {
		if got := Redact(in); got != in {
			t.Errorf("Redact(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestRedactAroundMovesTheMatch(t *testing.T) {
	text := "run TOKEN=abcdefgh now"
	for _, tc := range []struct {
		match, want string
	}{
		{"run", "run"},
		{"now", "now"},
		{"cdef", "[redacted]"},
		{"TOKEN=abc", "TOKEN=[redacted]"},
		{"gh now", "[redacted] now"},
	} {
		start := strings.Index(text, tc.match)
		out, s, e := redactAround(text, start, start+len(tc.match))
		if out != "run TOKEN=[redacted] now" || out[s:e] != tc.want {
			t.Errorf("%q: %q[%d:%d], want %q", tc.match, out, s, e, tc.want)
		}
	}
}
