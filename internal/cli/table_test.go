package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/style"
)

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
			want := map[string]string{"": style.Middle(path, maxDir), "middle": style.Middle(path, maxDir), "fish": style.Fish(path, maxDir), "full": path}[how]
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
		if out := runTable(t, h, deep, args); !strings.Contains(out, deep) {
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
		for _, width := range []int{rest + style.Width(short) + minLast + 5, rest + 30 + minLast, 40} {
			dir, trailing, _ := cells(width)
			wantDir := min(style.Width(short), max(width-rest-minLast, minDir))
			if style.Width(dir) > wantDir || wantDir == style.Width(short) && dir != short {
				t.Errorf("%v width %d: DIR %q, want %d columns at most", tc.args, width, dir, wantDir)
			}
			// cut drops the space before its …, so a cell can end a column short.
			if w := style.Width(trailing); w < minLast-1 || w > max(width-rest-style.Width(dir), minLast) {
				t.Errorf("%v width %d: %s %q is %d columns", tc.args, width, tc.trailer, trailing, w)
			}
		}
	}
}

// TestTablesPutPathsThroughDirCell reads the package's source, so a new table
// cannot show a path that the terminal's width and output.dirs leave alone.
// Only table.go calls style.Table, homeShort runs only there and in
// completions, and a table cell that names a path must be dirCell's. Key and
// value lists, which print directories whole for scripts, use tabwriter.
func TestTablesPutPathsThroughDirCell(t *testing.T) {
	allowed := map[string][]string{
		"style.Table":          {"table.go"},
		"homeShort":            {"table.go", "session.go"}, // session.go: completion descriptions
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
				if n.Body != nil && callsWriteTable(n.Body) {
					for _, cell := range tableCells(n.Body) {
						if namesPath(cell, pathNames) {
							use("path in a table cell", cell.Pos())
						}
					}
				}
			}
			return true
		})
	}
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

// namesPath reports whether the cell e shows something named as a path, such
// as r.Cwd or Faint(r.Cwd), other than through dirCell.
func namesPath(e ast.Expr, names []string) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return slices.Contains(names, e.Name)
	case *ast.SelectorExpr:
		return slices.Contains(names, e.Sel.Name)
	case *ast.ParenExpr:
		return namesPath(e.X, names)
	case *ast.BinaryExpr:
		return namesPath(e.X, names) || namesPath(e.Y, names)
	case *ast.CallExpr:
		if id, ok := e.Fun.(*ast.Ident); ok && id.Name == "dirCell" {
			return false
		}
		return slices.ContainsFunc(e.Args, func(a ast.Expr) bool { return namesPath(a, names) })
	}
	return false
}
