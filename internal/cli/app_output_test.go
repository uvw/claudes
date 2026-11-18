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

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
	"github.com/uvw/claudes/internal/render"
)

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
	{"session", "ls"}, {"session", "ls", "--wide"}, {"session", "where", "deep*"}, {"session", "du"}, {"session", "du", "--top", "3"},
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
			want := map[string]string{"": render.Middle(path, render.MaxDir), "middle": render.Middle(path, render.MaxDir), "fish": render.Fish(path, render.MaxDir), "full": path}[how]
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
				if w := render.Width(strings.TrimRight(line, "\n")); w > width {
					t.Errorf("width %d, %s: a line of %d columns:\n%s", width, args, w, out)
				}
			}
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
					rest = render.Width(strings.TrimRight(line, "\n")) - render.Width(dir) - render.Width(trailing)
				}
			}
			return dir, trailing, rest
		}
		dir, trailing, rest := cells(500)
		if dir != short || strings.HasSuffix(trailing, "…") {
			t.Fatalf("%v: a wide terminal cut %q or %q", tc.args, dir, trailing)
		}
		for _, width := range []int{rest + render.Width(short) + render.MinLast + 5, rest + 30 + render.MinLast, 40} {
			dir, trailing, _ := cells(width)
			wantDir := min(render.Width(short), max(width-rest-render.MinLast, render.MinDir))
			if render.Width(dir) > wantDir || wantDir == render.Width(short) && dir != short {
				t.Errorf("%v width %d: DIR %q, want %d columns at most", tc.args, width, dir, wantDir)
			}
			// cut drops the space before its …, so a cell can end a column short.
			if w := render.Width(trailing); w < render.MinLast-1 || w > max(width-rest-render.Width(dir), render.MinLast) {
				t.Errorf("%v width %d: %s %q is %d columns", tc.args, width, tc.trailer, trailing, w)
			}
		}
	}
}

// TestTablesPutPathsThroughDirCell reads the package's source, so a new table
// cannot show a path that the terminal's width and output.dirs leave alone.
// Only app_output.go calls render.Table, render.HomeShort runs only there and
// in completions, and a table cell that names a path must be render.DirCell's.
// Key and value lists, which print directories whole for scripts, use
// tabwriter.
func TestTablesPutPathsThroughDirCell(t *testing.T) {
	allowed := map[string][]string{
		"render.Table":         {"app_output.go"},
		"render.HomeShort":     {"app_output.go", "app_complete.go"}, // app_complete.go: completion descriptions
		"tabwriter.NewWriter":  {"config_get.go", "doctor_env.go", "session_show.go"},
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
				t.Errorf("%s: %s, which only %v may have; a table shows a path through render.DirCell", fset.Position(at), what, allowed[what])
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				if fun, ok := n.Fun.(*ast.SelectorExpr); ok {
					if x, ok := fun.X.(*ast.Ident); ok {
						if what := x.Name + "." + fun.Sel.Name; allowed[what] != nil {
							use(what, n.Pos())
						}
					}
				}
			case *ast.FuncDecl:
				for _, cell := range writtenCells(n) {
					if shows(cell, pathNames, "DirCell") {
						use("path in a table cell", cell.Pos())
					}
				}
			}
			return true
		})
	}
}

// TestTablesPutBranchesThroughBranchCell reads the package's source, so a new
// table cannot show a git branch that render.BranchCell and the terminal's
// width leave whole.
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
				if shows(cell, []string{"Branch", "branch"}, "BranchCell") {
					t.Errorf("%s: a branch in a table cell; a table shows a branch through render.BranchCell", fset.Position(cell.Pos()))
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
// as r.Cwd or Faint(r.Cwd) for a path, other than through the render function
// named through.
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
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == through {
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "render" {
				return false
			}
		}
		return slices.ContainsFunc(e.Args, func(a ast.Expr) bool { return shows(a, names, through) })
	}
	return false
}

func TestVerboseLogsTheStepsToStderr(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	h.ok(t, "ls")
	if strings.Contains(h.err.String(), "level=DEBUG") {
		t.Fatalf("logged without -v:\n%s", h.err)
	}
	h.ok(t, "stop", "-v", "api")
	assertContains(t, h.err.String(), "level=DEBUG msg=\"took the run lock\"")
	assertContains(t, h.out.String(), "stopped")
}

func TestClaudesLogSetsTheLevel(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}

	t.Setenv("CLAUDES_LOG", "debug")
	h.ok(t, "ls")
	assertContains(t, h.err.String(), "level=DEBUG")

	h.err.Reset()
	t.Setenv("CLAUDES_LOG", "loud")
	h.ok(t, "ls")
	assertContains(t, h.err.String(), "CLAUDES_LOG must be debug, info, warn, or error")
}

// The harness writes to buffers, so each case is a pipe.
func TestColorVariablesOnAPipe(t *testing.T) {
	for _, tc := range []struct {
		env     []string
		colored bool
	}{
		{[]string{"CLICOLOR=1"}, false},
		{[]string{"CLICOLOR_FORCE=1"}, true},
		{[]string{"CLICOLOR_FORCE=0"}, false},
		{[]string{"CLICOLOR_FORCE="}, false},
		{[]string{"CLICOLOR=0", "CLICOLOR_FORCE=1"}, true},
		{[]string{"CLICOLOR_FORCE=1", "NO_COLOR=1"}, false},
		{[]string{"CLICOLOR=1", "NO_COLOR=1"}, false},
		{[]string{"FORCE_COLOR=1"}, true},
		{[]string{"FORCE_COLOR=0"}, false},
		{[]string{"CLICOLOR=0", "FORCE_COLOR=1"}, true},
		{[]string{"FORCE_COLOR=1", "NO_COLOR=1"}, false},
	} {
		t.Run(strings.Join(tc.env, " "), func(t *testing.T) {
			h := newHarness(t)
			lsWorld(t, h)
			h.env(tc.env...)
			h.ok(t, "ls")
			out := h.out.String()
			h.exits(t, exit.NoMatch, "where", "nobody")
			errOut := h.err.String()
			if !tc.colored {
				assertPlain(t, "stdout", out)
				assertPlain(t, "stderr", errOut)
				return
			}
			assertContains(t, out, h.app.colors.Green("busy"))
			if !strings.HasPrefix(errOut, "\x1b[31mclaudes: ") {
				t.Fatalf("the error is not red:\n%q", errOut)
			}
		})
	}
}

func TestJSONYAMLCompletionInitAndSkillAreNeverColored(t *testing.T) {
	for _, args := range [][]string{
		{"ls", "-o", "json"}, {"ls", "--json"}, {"ls", "-o", "yaml"}, {"where", "pinned-one", "asking", "-o", "json"},
		{"doctor", "-o", "yaml"}, {"session", "ls", "-o", "json"}, {"restart", "--dry-run", "-o", "json"},
		{"__complete", "ls", ""}, {"completion", "bash"}, {"init", "zsh"}, {"skill"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			h := newHarness(t)
			lsWorld(t, h)
			h.env("CLICOLOR_FORCE=1")
			h.run(args...)
			assertPlain(t, "stdout", h.out.String())
		})
	}
}

func TestTablesShareOneColumnOrderAndCutLongNames(t *testing.T) {
	h := newHarness(t)
	long := strings.Repeat("A long title Claude Code wrote ", 3)
	live := h.bg("live", "idle")
	live.Name = long
	h.transcriptAs(t, live.SessionID, long, live.Cwd, 2*time.Hour)
	h.agents.snapshots = [][]claude.Session{{live}}

	header := regexp.MustCompile(`^ID\s.*\sDIR\s+NAME(\s+(NOTE|DOING))?$`)
	name := regexp.MustCompile(regexp.QuoteMeta(strings.TrimSpace(long[:render.MaxName-1])+"…") + `(\s\s+\S+)?$`)
	for _, args := range [][]string{{"ls"}, {"ls", "--wide"}, {"session", "ls"}, {"session", "ls", "--wide"}, {"session", "du", "--top", "1"}, {"restart", "--dry-run", live.JobID}} {
		h.out.Reset()
		h.ok(t, args...)
		var headed, short bool
		for line := range strings.Lines(h.out.String()) {
			line = strings.TrimSpace(line)
			headed = headed || header.MatchString(line)
			if strings.Contains(line, long[:20]) {
				short = name.MatchString(line)
			}
		}
		if !headed || !short {
			t.Errorf("%v: want ID first, then DIR, a name cut to %d characters, and at most a note:\n%s", args, render.MaxName, h.out)
		}
	}
}
