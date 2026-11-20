package layout

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// twoWords are names that join two words. A name of lowercase letters can
// still be two words, which no pattern tells apart, so this lists the ones
// already found, to keep them from coming back.
var twoWords = []string{
	"buildinfo", "claudejson", "exitcode", "flagval", "runlock", "safefile", "selfupdate",
}

// pkg is one of the module's packages: its name, its directory relative to the
// module root, and its files.
type pkg struct {
	name, dir string
	files     []*ast.File
}

// packages parses every package in the module, skipping nested modules and
// files built only by hand, such as hack/notices.go.
func packages(t *testing.T) map[string]*pkg {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("no go.mod at the module root %s: %v", root, err)
	}
	pkgs := map[string]*pkg{}
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
		f, err := parser.ParseFile(fset, p, nil, parser.PackageClauseOnly|parser.ParseComments)
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
			pk = &pkg{dir: dir}
			pkgs[dir] = pk
		}
		if !strings.HasSuffix(f.Name.Name, "_test") {
			pk.name = f.Name.Name
		}
		pk.files = append(pk.files, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pkgs
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

// docComment is a package doc comment's start: "Package name" and a word.
var docComment = regexp.MustCompile(`^Package [a-z]+ \S`)

func TestEveryPackageStatesItsConcern(t *testing.T) {
	for name, p := range internal(packages(t)) {
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

var oneWord = regexp.MustCompile(`^[a-z]+$`)

func TestPackageNamesAreOneWord(t *testing.T) {
	for name, p := range internal(packages(t)) {
		bad := !oneWord.MatchString(name) || slices.Contains(twoWords, name)
		switch {
		case p.name != name:
			t.Errorf("%s is package %s: name the package for its directory", p.dir, p.name)
		case bad:
			t.Errorf("%s is not one lowercase word: name it for its concern in one", p.dir)
		}
	}
}
