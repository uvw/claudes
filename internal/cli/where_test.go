package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
)

func TestWhereOneMatchPrintsTheBareFolder(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a, h.bg("bravo", "idle")}}

	h.ok(t, "where", "alpha")

	if h.out.String() != a.Cwd+"\n" {
		t.Fatalf("output %q", h.out.String())
	}
}

func TestWherePatternPrintsEveryMatch(t *testing.T) {
	h := newHarness(t)
	a, b := h.bg("plan-a", "idle"), h.bg("plan-b", "idle")
	h.agents.snapshots = [][]claude.Session{{a, b, h.bg("other", "idle")}}

	h.ok(t, "where", "plan*")

	lines := strings.Split(strings.TrimSpace(h.out.String()), "\n")
	if len(lines) != 3 || !regexp.MustCompile(`^ID +DIR +NAME$`).MatchString(lines[0]) ||
		!strings.HasPrefix(lines[1], a.ID()) || !strings.HasSuffix(lines[1], "plan-a") || !strings.Contains(lines[1], style.Middle(homeShort(a.Cwd), style.MaxDir)) ||
		!strings.HasPrefix(lines[2], b.ID()) || !strings.HasSuffix(lines[2], "plan-b") || !strings.Contains(lines[2], style.Middle(homeShort(b.Cwd), style.MaxDir)) {
		t.Fatalf("output:\n%s", h.out)
	}
}

func TestWhereJSON(t *testing.T) {
	h := newHarness(t)
	a := h.bg("alpha", "idle")
	h.agents.snapshots = [][]claude.Session{{a}}

	h.ok(t, "where", "alpha", "-o", "json")

	fromWhere := h.out.String()
	h.out.Reset()
	h.ok(t, "ls", "alpha", "-o", "json")
	if fromWhere != h.out.String() {
		t.Fatalf("where:\n%s\nls:\n%s", fromWhere, h.out)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(fromWhere), &got); err != nil || len(got) != 1 {
		t.Fatalf("got %v, err %v", got, err)
	}
	if got[0]["sessionId"] != a.SessionID || got[0]["cwd"] != a.Cwd {
		t.Fatalf("got %v", got[0])
	}
}
