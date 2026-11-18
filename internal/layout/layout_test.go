package layout

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/uvw/claudes"

// allowed is every import one of the module's packages may make of another,
// by directory. Packages import downward only: cmd/claudes imports cli, cli
// imports the packages of one concern each, and those import the leaf packages.
// Nothing imports cli. A new import, or one a package no longer makes, changes
// this map, so each shows up in review.
var allowed = map[string][]string{
	"cmd/claudes":     {"internal/cli"},
	"internal/claude": {"internal/shell"},
	"internal/cli": {
		"internal/claude", "internal/compat", "internal/config", "internal/exit",
		"internal/files", "internal/install", "internal/interrupt", "internal/jobs", "internal/mcp", "internal/park",
		"internal/paths", "internal/procs", "internal/shell", "internal/style",
		"internal/target", "internal/teams", "internal/transcripts", "internal/trash",
		"internal/upgrade", "internal/version", "skills",
	},
	"internal/compat":        {"internal/claude", "internal/files", "internal/jobs", "internal/version"},
	"internal/config":        {"internal/files", "internal/paths", "internal/style"},
	"internal/exit":          {"internal/target"},
	"internal/files":         nil,
	"internal/install":       {"internal/files", "internal/paths"},
	"internal/interrupt":     nil,
	"internal/jobs":          {"internal/files", "internal/paths"},
	"internal/layout":        nil,
	"internal/mcp":           {"internal/paths"},
	"internal/park":          {"internal/claude", "internal/files", "internal/paths", "internal/shell"},
	"internal/paths":         nil,
	"internal/procs":         nil,
	"internal/shell":         nil,
	"internal/style":         nil,
	"internal/target":        nil,
	"internal/teams":         {"internal/claude", "internal/paths"},
	"internal/transcripts":   {"internal/claude", "internal/files", "internal/paths", "internal/shell"},
	"internal/trash":         {"internal/files"},
	"internal/upgrade":       nil,
	"internal/version":       nil,
	"skills":                 nil,
	"test/e2e":               {"test/e2e/internal/fake"},
	"test/e2e/fakeclaude":    {"test/e2e/internal/fake"},
	"test/e2e/internal/fake": nil,
}

// notYetNamed are the packages AGENTS.md does not name yet.
var notYetNamed = []string{"version"}

// notYetRenamed are the package names that join two words, with the one word
// each takes.
var notYetRenamed = map[string]string{}

// twoWords are names that join two words. A name of lowercase letters can
// still be two words, which no pattern tells apart, so this lists the ones
// already found, to keep them from coming back.
var twoWords = []string{
	"buildinfo", "claudejson", "exitcode", "flagval", "runlock", "safefile", "selfupdate",
}

// pkg is one of the module's packages: its name, its directory relative to the
// module root, its files, and what it imports from the module.
type pkg struct {
	name, dir string
	files     []*ast.File
	imports   map[string]bool
}

// packages parses every package in the module, skipping nested modules and
// files built only by hand, such as hack/notices.go.
func packages(t *testing.T) (root string, pkgs map[string]*pkg) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at the module root %s: %v", root, err)
	}
	pkgs = map[string]*pkg{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
				name == "testdata" || name == "node_modules" || name == "dist") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); p != root && err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		if handBuilt(f) {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		dir := filepath.ToSlash(rel)
		pk := pkgs[dir]
		if pk == nil {
			pk = &pkg{dir: dir, imports: map[string]bool{}}
			pkgs[dir] = pk
		}
		if !strings.HasSuffix(f.Name.Name, "_test") {
			pk.name = f.Name.Name
		}
		pk.files = append(pk.files, f)
		for _, imp := range f.Imports {
			ipath, _ := strconv.Unquote(imp.Path.Value)
			if dep, ok := strings.CutPrefix(ipath, module+"/"); ok && dep != dir {
				pk.imports[dep] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, pkgs
}

// handBuilt reports whether f carries //go:build ignore, so no build includes
// it.
func handBuilt(f *ast.File) bool {
	for _, group := range f.Comments {
		if group.Pos() >= f.Package {
			break
		}
		for _, c := range group.List {
			if strings.TrimSpace(c.Text) == "//go:build ignore" {
				return true
			}
		}
	}
	return false
}

// internal is the packages directly under internal/, by name.
func internal(pkgs map[string]*pkg) map[string]*pkg {
	out := map[string]*pkg{}
	for dir, p := range pkgs {
		if path.Dir(dir) == "internal" {
			out[path.Base(dir)] = p
		}
	}
	return out
}

func TestImportsGoDownward(t *testing.T) {
	_, pkgs := packages(t)
	for _, dir := range slices.Sorted(maps.Keys(pkgs)) {
		want, ok := allowed[dir]
		if !ok {
			t.Errorf("%s is not in allowed: add it, with the packages it imports", dir)
			continue
		}
		for _, dep := range slices.Sorted(maps.Keys(pkgs[dir].imports)) {
			switch {
			case dep == "internal/cli" && dir != "cmd/claudes":
				t.Errorf("%s imports internal/cli, which only cmd/claudes may import", dir)
			case !slices.Contains(want, dep):
				t.Errorf("%s imports %s, which allowed does not list: import downward, or add it to allowed", dir, dep)
			}
		}
		for _, dep := range want {
			if !pkgs[dir].imports[dep] {
				t.Errorf("%s no longer imports %s: take it off allowed", dir, dep)
			}
		}
	}
	for dir := range allowed {
		if pkgs[dir] == nil {
			t.Errorf("allowed lists %s, which has no Go files: take it off", dir)
		}
	}
}

// docComment is a package doc comment's start: "Package name" and a word.
var docComment = regexp.MustCompile(`^Package [a-z]+ \S`)

func TestEveryPackageStatesItsConcern(t *testing.T) {
	_, pkgs := packages(t)
	for name, p := range internal(pkgs) {
		var docs []string
		for _, f := range p.files {
			if f.Doc != nil && !strings.HasSuffix(f.Name.Name, "_test") {
				docs = append(docs, f.Doc.Text())
			}
		}
		switch {
		case len(docs) == 0:
			t.Errorf("%s has no package doc comment: say its one concern in one", p.dir)
		case len(docs) > 1:
			t.Errorf("%s has %d package doc comments: keep the one that says its concern", p.dir, len(docs))
		case !docComment.MatchString(docs[0]) || !strings.HasPrefix(docs[0], "Package "+name+" "):
			t.Errorf("%s's doc comment starts %q, not \"Package %s\" and its concern", p.dir, strings.SplitN(docs[0], "\n", 2)[0], name)
		}
	}
}

func TestAgentsNamesEveryPackage(t *testing.T) {
	root, pkgs := packages(t)
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range internal(pkgs) {
		named := regexp.MustCompile(`\b` + regexp.QuoteMeta(p.dir) + `\b`).Match(agents)
		waiting := slices.Contains(notYetNamed, name)
		switch {
		case !named && !waiting:
			t.Errorf("AGENTS.md does not name %s: add it to \"Where to look first\"", p.dir)
		case named && waiting:
			t.Errorf("AGENTS.md names %s now: take it off notYetNamed", p.dir)
		}
	}
	for _, name := range notYetNamed {
		if internal(pkgs)[name] == nil {
			t.Errorf("notYetNamed lists %s, which is not a package: take it off", name)
		}
	}
}

var oneWord = regexp.MustCompile(`^[a-z]+$`)

func TestPackageNamesAreOneWord(t *testing.T) {
	_, pkgs := packages(t)
	for name, p := range internal(pkgs) {
		_, waiting := notYetRenamed[name]
		bad := !oneWord.MatchString(name) || slices.Contains(twoWords, name)
		switch {
		case p.name != name:
			t.Errorf("%s is package %s: name the package for its directory", p.dir, p.name)
		case bad && !waiting:
			t.Errorf("%s is not one lowercase word: name it for its concern in one", p.dir)
		case !bad && waiting:
			t.Errorf("%s is one word now: take it off notYetRenamed", p.dir)
		}
	}
	for name := range notYetRenamed {
		if internal(pkgs)[name] == nil {
			t.Errorf("notYetRenamed lists %s, which is not a package: take it off", name)
		}
	}
}
