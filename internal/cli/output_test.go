package cli

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.yaml.in/yaml/v4"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
)

func TestLsNarrowsToTheTargets(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("plan-a", "idle"), h.bg("plan-b", "idle")}}

	h.ok(t, "ls", "plan*")

	out := h.out.String()
	if !strings.Contains(out, "plan-a") || !strings.Contains(out, "plan-b") || strings.Contains(out, "alpha") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestLsPrintsASessionOnceWhenTwoTargetsMatchIt(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("plan-a", "idle")}}

	h.ok(t, "ls", "plan*", "plan-a", "-o", "json")

	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("rows %v, err %v", rows, err)
	}
}

func TestLsYAMLAndJSONHaveTheSameFields(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.tty("terminal", "busy")}}

	h.ok(t, "ls", "-o", "json")
	var fromJSON []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &fromJSON); err != nil {
		t.Fatal(err)
	}
	h.out.Reset()
	h.ok(t, "ls", "-o", "yaml")
	var fromYAML []map[string]any
	if err := yaml.Unmarshal(h.out.Bytes(), &fromYAML); err != nil {
		t.Fatalf("%v:\n%s", err, h.out)
	}
	if strings.Contains(h.out.String(), "{") {
		t.Fatalf("yaml is not in block style:\n%s", h.out)
	}
	if len(fromJSON) != 2 {
		t.Fatalf("json %v", fromJSON)
	}
	if diff := cmp.Diff(normalize(t, fromJSON), normalize(t, fromYAML)); diff != "" {
		t.Fatalf("yaml differs from json (-json +yaml):\n%s", diff)
	}
}

// normalize round-trips through JSON so YAML's ints compare equal to JSON's
// floats.
func normalize(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLsJSONFlagStillWorks(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
	h.ok(t, "ls", "--json")
	short := h.out.String()
	h.out.Reset()
	h.ok(t, "ls", "-o", "json")
	if short != h.out.String() || !strings.HasPrefix(short, "[") {
		t.Fatalf("--json:\n%s\n-o json:\n%s", short, h.out)
	}
}

func TestUnknownOutputIsAUsageError(t *testing.T) {
	for _, args := range [][]string{{"ls", "-o", "xml"}, {"where", "alpha", "-o", "xml"}} {
		h := newHarness(t)
		h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle")}}
		h.fails(t, args...)
		assertContains(t, h.err.String(), "must be text, json, or yaml")
		if h.out.Len() != 0 {
			t.Fatalf("%v printed:\n%s", args, h.out)
		}
	}
}

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

func TestJSONHasEveryKeyForEverySession(t *testing.T) {
	h := newHarness(t)
	term := h.tty("terminal", "idle")
	term.Name = ""
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "busy"), term}}
	h.agents.finished = []claude.Session{stopped(h.bg("gone", ""))}

	h.ok(t, "ls", "-a", "-o", "json")

	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("rows %v, err %v", rows, err)
	}
	keys := []string{"id", "sessionId", "name", "cwd", "kind", "state", "status", "pid", "startedAt", "pinned", "version", "outdated", "teammates", "subagents", "shells", "tokens", "prs", "doing"}
	for _, r := range rows {
		if len(r) != len(keys) {
			t.Errorf("%v has %d keys, want %d", r, len(r), len(keys))
		}
		for _, k := range keys {
			if _, ok := r[k]; !ok {
				t.Errorf("%v has no %q", r, k)
			}
		}
	}
}

func TestWhereAndLsCompleteMoreThanOneName(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("alpha", "idle"), h.bg("bravo", "idle")}}

	assertOffers(t, h.complete(t, "where", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "alpha", ""), "bravo")
	assertOffers(t, h.complete(t, "ls", "-o", ""), "text", "json", "yaml")
}

func TestYAMLQuotesOnlyWhatNeedsIt(t *testing.T) {
	var buf strings.Builder
	type item struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		Cwd  string `json:"cwd"`
	}
	err := (&outputFlag{"yaml"}).encode(&buf, []item{{Name: "alpha", ID: "12345678", Cwd: `C:\x`}})
	if err != nil {
		t.Fatal(err)
	}
	want := "- name: alpha\n  id: \"12345678\"\n  cwd: C:\\x\n"
	if buf.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

func TestLsShowsWhatSomeTargetsMatchAndNotesTheRest(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle"), h.bg("docs", "idle")}}

	h.ok(t, "ls", "api", "nothing", "-o", "json")
	var rows []map[string]any
	if err := json.Unmarshal(h.out.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0]["name"] != "api" {
		t.Fatalf("rows %v, err %v\n%s", rows, err, h.out)
	}
	assertContains(t, h.err.String(), `no session matches "nothing"`)
}

// deepHarness is a harness whose home is its root, with two background
// sessions, deep-a and deep-b, that ran in a directory too deep for a table,
// and a past session, an agent team, an MCP server, and leftovers there.
func deepHarness(t *testing.T) (h *harness, deep, short string) {
	h = newHarness(t)
	t.Setenv("HOME", h.root)
	t.Setenv("USERPROFILE", h.root)
	deep = filepath.Join(h.root, "work", "acme", "monorepo", "services", "billing", "internal", "ledger", "testdata", "v2")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	a, b := h.bg("deep-a", "busy"), h.bg("deep-b", "idle")
	a.Cwd, b.Cwd = deep, deep
	h.agents.snapshots = [][]claude.Session{{a, b}}
	writeActivity(t, h, a, 0, 0, 0, fmt.Sprintf(`,"detail":%q`, strings.Repeat("checking the build on every platform ", 4)))
	h.transcriptAs(t, a.SessionID, "deep-a", deep, time.Hour)
	h.transcriptAs(t, b.SessionID, "deep-b", deep, 2*time.Hour)
	h.said(t, "zebra", deep, 48*time.Hour, typed("please feed the zebra, then water every plant in the greenhouse twice"))
	h.sessionOf(t, "ping", deep, 7*24*time.Hour, prompt("ping"), reply)
	h.app.Teams.Teams = filepath.Join(deep, "teams")
	h.team(t, a, "writer")
	old := h.now.Add(-30 * 24 * time.Hour)
	leftover := filepath.Join(h.app.Teams.Teams, "session-aaaaaaaa", "config.json")
	writeFile(t, leftover, "{}")
	if err := os.Chtimes(leftover, old, old); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(deep, ".mcp.json"), `{"mcpServers": {"ledger-tool": {"command": "x"}}}`)
	return h, deep, "~" + string(filepath.Separator) + must(filepath.Rel(h.root, deep))
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// tableCommands are the commands whose text output shows a directory in a
// table or a heading.
var tableCommands = [][]string{
	{"ls"}, {"ls", "--wide"}, {"where", "deep*"}, {"team", "ls"},
	{"restart", "--dry-run", "deep-b"}, {"gc", "--teams", "--dry-run"},
	{"session", "ls"}, {"session", "where", "deep*"}, {"session", "du"}, {"session", "du", "--top", "3"},
	{"session", "search", "greenhouse"}, {"session", "triage"}, {"session", "gc", "--trivial", "--dry-run"},
	{"mcp", "ls", "--in"},
}

func runTable(t *testing.T, h *harness, deep string, args []string) string {
	t.Helper()
	if args[len(args)-1] == "--in" {
		args = append(slices.Clip(args), deep)
	}
	h.out.Reset()
	h.ok(t, args...)
	return h.out.String()
}

func TestEveryTableShortensADeepDirectory(t *testing.T) {
	h, deep, short := deepHarness(t)
	for _, how := range []string{"", "middle", "fish", "full"} {
		h.app.Config.Output.Dirs = how
		for _, args := range tableCommands {
			path := short
			if args[0] == "gc" {
				path = filepath.Join(short, "teams", "session-aaaaaaaa")
			}
			want := map[string]string{"": style.Middle(path, style.MaxDir), "middle": style.Middle(path, style.MaxDir), "fish": style.Fish(path, style.MaxDir), "full": path}[how]
			out := runTable(t, h, deep, args)
			if !strings.Contains(out, want) {
				t.Errorf("%q %v: want %s:\n%s", how, args, want, out)
			}
			if how != "full" && (strings.Contains(out, short) || strings.Contains(out, deep)) {
				t.Errorf("%q %v: shows the whole directory:\n%s", how, args, out)
			}
		}
	}
}

func TestJSONAndKeyValueOutputKeepTheWholeDirectory(t *testing.T) {
	h, deep, _ := deepHarness(t)
	for _, args := range [][]string{
		{"ls", "-o", "json"}, {"where", "deep-a"}, {"session", "where", "deep-a"},
		{"session", "ls", "-o", "yaml"}, {"session", "show", "deep-a"},
	} {
		// JSON writes each backslash of a Windows path as two.
		escaped := strings.Trim(string(must(json.Marshal(deep))), `"`)
		if out := runTable(t, h, deep, args); !strings.Contains(out, deep) && !strings.Contains(out, escaped) {
			t.Errorf("%v: want the whole directory %s:\n%s", args, deep, out)
		}
	}
}

func TestDirectoriesFitTheTerminal(t *testing.T) {
	h, deep, short := deepHarness(t)
	h.app.Width = func() int { return 500 }
	for _, args := range tableCommands {
		if out := runTable(t, h, deep, args); !strings.Contains(out, short) {
			t.Errorf("%v: a wide terminal does not show the whole directory:\n%s", args, out)
		}
	}

	for args, widths := range map[string][]int{"ls": {100, 110}, "session ls": {80, 100}, "session du": {80, 90}, "team ls": {70, 90}} {
		for _, width := range widths {
			h.app.Width = func() int { return width }
			out := runTable(t, h, deep, strings.Fields(args))
			if !strings.Contains(out, "…") {
				t.Errorf("width %d, %s: nothing shortened:\n%s", width, args, out)
			}
			for line := range strings.Lines(out) {
				if w := style.Width(strings.TrimRight(line, "\n")); w > width {
					t.Errorf("width %d, %s: a line of %d columns:\n%s", width, args, w, out)
				}
			}
		}
	}
}

func TestSessionLsFitsALongBranchAndNameToTheTerminal(t *testing.T) {
	h, deep, _ := deepHarness(t)
	branch, name := "feat/ledger-rounding-for-every-currency", "reconcile the ledger totals for each month"[:maxName]
	h.said(t, name, deep, time.Hour, fmt.Sprintf(`{"type":"user","gitBranch":%q,"message":{"content":"go"}}`, branch))
	cells := func(out string) (string, string) {
		for n, b := range column(t, out, "BRANCH") {
			if strings.HasPrefix(n, "reconcile") {
				return b, n
			}
		}
		t.Fatalf("no row for %q:\n%s", name, out)
		return "", ""
	}

	out := runTable(t, h, deep, []string{"session", "ls"})
	if b, n := cells(out); b != style.Branch(branch, maxBranch) || n != name {
		t.Errorf("off a terminal: BRANCH %q and NAME %q, want %q and the whole name:\n%s", b, n, style.Branch(branch, maxBranch), out)
	}
	for _, width := range []int{90, 100, 120} {
		h.app.Width = func() int { return width }
		out := runTable(t, h, deep, []string{"session", "ls"})
		for line := range strings.Lines(out) {
			if w := style.Width(strings.TrimRight(line, "\n")); w > width {
				t.Errorf("width %d: a line of %d columns:\n%s", width, w, out)
			}
		}
		// cut drops the space before its …, so a name can end a column short.
		if b, n := cells(out); style.Width(b) < minBranch || style.Width(n) < minName-1 || style.Width(n) >= maxName {
			t.Errorf("width %d: BRANCH %q, NAME %q:\n%s", width, b, n, out)
		}
	}
}

func TestADirectoryAndADoingOrMatchColumnShareTheLine(t *testing.T) {
	h, deep, short := deepHarness(t)
	for _, tc := range []struct {
		args         []string
		trailer, row string
	}{
		{[]string{"ls", "--wide"}, "DOING", "deep-a"},
		{[]string{"session", "search", "greenhouse"}, "MATCH", "zebra"},
	} {
		// cells is the row's DIR and trailing cells, and how wide the rest of
		// its line is.
		cells := func(width int) (dir, trailing string, rest int) {
			h.app.Width = func() int { return width }
			out, _, _ := strings.Cut(runTable(t, h, deep, tc.args), "\n\n")
			dir, trailing = column(t, out, "DIR")[tc.row], column(t, out, tc.trailer)[tc.row]
			for line := range strings.Lines(out) {
				if strings.Contains(line, tc.row) {
					rest = style.Width(strings.TrimRight(line, "\n")) - style.Width(dir) - style.Width(trailing)
				}
			}
			return dir, trailing, rest
		}
		dir, trailing, rest := cells(500)
		if dir != short || strings.HasSuffix(trailing, "…") {
			t.Fatalf("%v: a wide terminal cut %q or %q", tc.args, dir, trailing)
		}
		for _, width := range []int{rest + style.Width(short) + style.MinLast + 5, rest + 30 + style.MinLast, 40} {
			dir, trailing, _ := cells(width)
			wantDir := min(style.Width(short), max(width-rest-style.MinLast, style.MinDir))
			if style.Width(dir) > wantDir || wantDir == style.Width(short) && dir != short {
				t.Errorf("%v width %d: DIR %q, want %d columns at most", tc.args, width, dir, wantDir)
			}
			// cut drops the space before its …, so a cell can end a column short.
			if w := style.Width(trailing); w < style.MinLast-1 || w > max(width-rest-style.Width(dir), style.MinLast) {
				t.Errorf("%v width %d: %s %q is %d columns", tc.args, width, tc.trailer, trailing, w)
			}
		}
	}
}

// TestTablesPutPathsThroughDirCell reads the package's source, so a new table
// cannot show a path that the terminal's width and output.dirs leave alone.
// Only output.go calls style.Table, homeShort runs only there and in
// completions, and a table cell that names a path must be dirCell's. Key and
// value lists, which print directories whole for scripts, use tabwriter.
func TestTablesPutPathsThroughDirCell(t *testing.T) {
	allowed := map[string][]string{
		"style.Table":          {"output.go"},
		"homeShort":            {"output.go", "session.go"}, // session.go: completion descriptions
		"tabwriter.NewWriter":  {"config.go", "doctor_env.go", "session_show.go"},
		"path in a table cell": {},
	}
	pathNames := []string{"Cwd", "Dir", "Folder", "Path", "cwd", "dir", "folder", "path"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		use := func(what string, at token.Pos) {
			if !slices.Contains(allowed[what], name) {
				t.Errorf("%s: %s, which only %v may have; a table shows a path through dirCell", fset.Position(at), what, allowed[what])
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				switch fun := n.Fun.(type) {
				case *ast.Ident:
					if fun.Name == "homeShort" {
						use("homeShort", n.Pos())
					}
				case *ast.SelectorExpr:
					if x, ok := fun.X.(*ast.Ident); ok && (x.Name == "style" && fun.Sel.Name == "Table" || x.Name == "tabwriter" && fun.Sel.Name == "NewWriter") {
						use(x.Name+"."+fun.Sel.Name, n.Pos())
					}
				}
			case *ast.FuncDecl:
				for _, cell := range writtenCells(n) {
					if shows(cell, pathNames, "dirCell") {
						use("path in a table cell", cell.Pos())
					}
				}
			}
			return true
		})
	}
}

// TestTablesPutBranchesThroughBranchCell reads the package's source, so a new
// table cannot show a git branch that branchCell and the terminal's width
// leave whole.
func TestTablesPutBranchesThroughBranchCell(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			for _, cell := range writtenCells(decl) {
				if shows(cell, []string{"Branch", "branch"}, "branchCell") {
					t.Errorf("%s: a branch in a table cell; a table shows a branch through branchCell", fset.Position(cell.Pos()))
				}
			}
		}
	}
}

// writtenCells is every table cell of decl, when it is a function that writes
// a table.
func writtenCells(decl ast.Decl) []ast.Expr {
	if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && callsWriteTable(fn.Body) {
		return tableCells(fn.Body)
	}
	return nil
}

// callsWriteTable reports whether body writes a table.
func callsWriteTable(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "writeTable" {
			found = true
		}
		return !found
	})
	return found
}

// tableCells is every element of a []string literal in body, and every value
// appended to a slice.
func tableCells(body *ast.BlockStmt) []ast.Expr {
	var cells []ast.Expr
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if a, ok := n.Type.(*ast.ArrayType); ok {
				if id, ok := a.Elt.(*ast.Ident); ok && id.Name == "string" {
					cells = append(cells, n.Elts...)
				}
			}
		case *ast.CallExpr:
			if id, ok := n.Fun.(*ast.Ident); ok && id.Name == "append" && len(n.Args) > 1 {
				cells = append(cells, n.Args[1:]...)
			}
		}
		return true
	})
	return cells
}

// shows reports whether the cell e shows something with one of names, such
// as r.Cwd or Faint(r.Cwd) for a path, other than through the function named
// through.
func shows(e ast.Expr, names []string, through string) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return slices.Contains(names, e.Name)
	case *ast.SelectorExpr:
		return slices.Contains(names, e.Sel.Name)
	case *ast.ParenExpr:
		return shows(e.X, names, through)
	case *ast.BinaryExpr:
		return shows(e.X, names, through) || shows(e.Y, names, through)
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == through {
			return false
		}
		return slices.ContainsFunc(e.Args, func(a ast.Expr) bool { return shows(a, names, through) })
	}
	return false
}

func TestHomeShort(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sep := string(filepath.Separator)
	for path, want := range map[string]string{
		home:                                  "~",
		filepath.Join(home, "work", "repo"):   "~" + sep + filepath.Join("work", "repo"),
		home + "-other":                       home + "-other",
		filepath.Join(os.TempDir(), "x", "y"): filepath.Join(os.TempDir(), "x", "y"),
	} {
		if got := homeShort(path); got != want {
			t.Errorf("%s: got %s, want %s", path, got, want)
		}
	}
}
