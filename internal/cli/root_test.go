package cli

import (
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/uvw/claudes/internal/claude"
	"github.com/uvw/claudes/internal/exit"
)

// inParentsFile are the commands whose code lives in another command's file,
// each with why.
var inParentsFile = map[string]string{
	"claudes unpin": "pinCommand builds pin and unpin, which differ only in what they write",
}

// TestEachCommandLivesInItsFile walks the command tree and finds the file of
// each command's RunE, so a command's code is where its name says:
// session_mv.go for claudes session mv.
func TestEachCommandLivesInItsFile(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			walk(c)
		}
		if cmd.RunE == nil || !cmd.HasParent() {
			return
		}
		file, _ := runtime.FuncForPC(reflect.ValueOf(cmd.RunE).Pointer()).FileLine(reflect.ValueOf(cmd.RunE).Pointer())
		if dir, _ := filepath.Abs("."); filepath.Dir(file) != dir {
			return // cobra's own, such as help and completion
		}
		path := cmd.CommandPath()
		want := strings.ReplaceAll(strings.TrimPrefix(path, "claudes "), " ", "_") + ".go"
		if filepath.Base(file) == want {
			if _, ok := inParentsFile[path]; ok {
				t.Errorf("%s is in %s now: take it off inParentsFile", path, want)
			}
			return
		}
		if _, ok := inParentsFile[path]; !ok {
			t.Errorf("%s runs from %s, not %s: move it, or add it to inParentsFile with why", path, filepath.Base(file), want)
		}
	}
	walk(newHarness(t).app.rootCommand())
}

// docs/reference.md: exit 4 is "Refused before any change: the wrong kind of
// session, or a failed safety check".
func TestSafetyRefusalsExitFour(t *testing.T) {
	h := newHarness(t)
	host := h.bg("host", "busy")
	live := h.bg("live", "busy")
	h.app.HostID = host.SessionID
	h.agents.snapshots = [][]claude.Session{{host, live}}
	h.transcriptAs(t, live.SessionID, "live", live.Cwd, time.Minute)
	for _, args := range [][]string{
		{"ping", "host", "hi"},
		{"session", "resume", "live"},
		{"session", "mv", "live", h.folder("other")},
		{"gc", "live"},
	} {
		os.MkdirAll(h.folder("other"), 0o755)
		h.err.Reset()
		if code := h.run(args...); code != exit.Refused {
			t.Errorf("%v exits %d: %s", args, code, strings.TrimSpace(h.err.String()))
		}
	}
}

func TestBuildVersionPrefersTheReleaseVersion(t *testing.T) {
	defer func(v, c string) { Version, Commit = v, c }(Version, Commit)
	Version, Commit = "0.3.0", "abc1234"
	if got := buildVersion(); got != "0.3.0 (abc1234)" {
		t.Fatalf("got %q", got)
	}
	Commit = ""
	if got := buildVersion(); got != "0.3.0" {
		t.Fatalf("got %q", got)
	}
}

// maxHelpWidth is the widest line help may print, so it reads in an
// 80-column terminal without wrapping.
const maxHelpWidth = 80

func TestHelpFitsEightyColumns(t *testing.T) {
	h := newHarness(t)
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			// cobra's own help and completion commands.
			if c.Name() != "help" && c.Name() != "completion" {
				walk(c)
			}
		}
		var b strings.Builder
		cmd.SetOut(&b)
		if err := cmd.Help(); err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(b.String()) {
			if line = strings.TrimRight(line, "\n"); utf8.RuneCountInString(line) > maxHelpWidth {
				t.Errorf("%s --help has a line of %d columns:\n%s", cmd.CommandPath(), utf8.RuneCountInString(line), line)
			}
		}
	}
	walk(h.app.rootCommand())
}

// appFile is the name of a file of plumbing the commands share.
var appFile = regexp.MustCompile(`^app_[a-z]+$`)

// notYetMoved are the files that are neither root, an app file, nor a
// command's.
var notYetMoved = []string{
	"cells.go", "cells_test.go", "config_edit_unix.go", "config_edit_windows.go",
	"dirs.go", "version.go",
}

// commandFiles are the names, without .go, that a command's file takes: its
// path below claudes, joined by underscores, such as session_mv.
func commandFiles(t *testing.T) map[string]bool {
	t.Helper()
	files := map[string]bool{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, c := range cmd.Commands() {
			// cobra's own help and completion commands.
			if c.Name() != "help" && c.Name() != "completion" {
				walk(c)
			}
		}
		if cmd.HasParent() {
			files[strings.ReplaceAll(strings.TrimPrefix(cmd.CommandPath(), "claudes "), " ", "_")] = true
		}
	}
	walk(newHarness(t).app.rootCommand())
	return files
}

func TestEveryFileIsACommandAnAppFileOrRoot(t *testing.T) {
	commands := commandFiles(t)
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		base, test := strings.CutSuffix(strings.TrimSuffix(name, ".go"), "_test")
		fits := base == "root" || base == "app" || appFile.MatchString(base) || commands[base] ||
			test && base == "harness"
		waiting := slices.Contains(notYetMoved, name)
		switch {
		case !fits && !waiting:
			t.Errorf("%s is not root, app, app_<concern>, or a command's file: "+
				"move its code to one of those, or to a package of its own concern", name)
		case fits && waiting:
			t.Errorf("%s fits now: take it off notYetMoved", name)
		}
	}
	for _, name := range notYetMoved {
		if !slices.Contains(names, name) {
			t.Errorf("notYetMoved lists %s, which is gone: take it off", name)
		}
	}
}

// parsed is this package's files, without its tests, by name.
func parsed(t *testing.T) map[string]*ast.File {
	t.Helper()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	fset := token.NewFileSet()
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = f
	}
	return files
}

func isApp(expr ast.Expr) bool {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "App"
}

func recvType(fn *ast.FuncDecl) string {
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if idx, ok := expr.(*ast.IndexExpr); ok {
		expr = idx.X
	}
	return expr.(*ast.Ident).Name
}

func takesApp(fn *ast.FuncDecl) bool {
	params := fn.Type.Params.List
	return len(params) > 0 && isApp(params[0].Type)
}

// declName is how the allow-lists name a function: its name, or Type.name for
// a method.
func declName(fn *ast.FuncDecl) string {
	if fn.Recv == nil {
		return fn.Name.Name
	}
	return recvType(fn) + "." + fn.Name.Name
}

func isPlumbing(name string) bool {
	base := strings.TrimSuffix(name, ".go")
	return base == "root" || base == "app" || appFile.MatchString(base)
}

// plumbingFuncs are the functions root and the app files may declare although
// they take no *App, each with why.
var plumbingFuncs = map[string]string{
	"Main":        "the entry point cmd/claudes calls",
	"newApp":      "builds the App that Main runs",
	"passThrough": "decides whether the arguments go to claude agents",
	"commandName": "finds the command among the arguments, for passThrough and Run",
	"fixesConfig": "names the commands that run with a broken config.yaml",
	"alone":       "marks a command that takes the run lock",
	"reports":     "marks a command that prints a report for -o json and -o yaml",
}

// plumbingTypes are the types other than App whose methods the app files may
// declare, each with why.
var plumbingTypes = map[string]string{
	"lifecycleFlags": "the flags of restart, pause, and resume",
	"planFlags":      "--dry-run and --confirm",
	"outputFlag":     "-o, which every command that prints data takes",
	"days":           "a flag value that also takes days, such as 7d",
	"countFlag":      "a flag value that takes a whole number",
	"choiceFlag":     "a flag value that takes one of a few words",
	"errWriter":      "writes the log to App.Err, which tests swap",
}

// notYetHelpers are the functions this package declares that belong in an app
// file as *App methods, or in a package of their own concern.
var notYetHelpers = []string{
	"activity", "age", "ago", "branchCell", "buildVersion", "checkFail",
	"checkOK", "checkWarn", "clearStyle", "completeKeys", "count", "countCell",
	"dash", "describeForCompletion", "dirCell", "dirGone", "doing", "duRow",
	"duTally", "duTop", "editFile", "entryOf", "firstLine",
	"firstPrompt", "gcCommandFor", "homeShort", "hostJob", "isFinished", "joinNotes", "keepWhy", "kind", "labels", "labelsOf",
	"leftovers", "managedBy", "nameCell", "nameWithSource", "named", "noProfile",
	"noteOf", "oneEach", "orList", "pickPast", "printableList", "readTally",
	"realDir", "refOf", "relWithin", "removeCommand",
	"requireFinished", "requireRunning", "resolved", "runningIn", "sameDir",
	"sameName", "sessionRef.Label", "sidecarItem", "sizeDetail", "skipSet",
	"stamp", "taskSummary", "tokenCount", "triageOf", "triageSize",
	"trivial", "trueFirst", "withOptions", "within", "zeroIfKnown",
}

func TestCLIDeclaresNoHelpers(t *testing.T) {
	var found []string
	for name, f := range parsed(t) {
		local := map[string]bool{}
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
				for _, spec := range gen.Specs {
					local[spec.(*ast.TypeSpec).Name.Name] = true
				}
			}
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			var fits bool
			switch {
			case fn.Recv != nil && isApp(fn.Recv.List[0].Type):
				fits = true
			case isPlumbing(name):
				if fn.Recv != nil {
					_, fits = plumbingTypes[recvType(fn)]
				} else {
					_, excepted := plumbingFuncs[fn.Name.Name]
					fits = takesApp(fn) || excepted
				}
			default:
				// A command file's own flag and output types may have methods.
				fits = fn.Recv != nil && local[recvType(fn)]
			}
			if fits {
				continue
			}
			found = append(found, declName(fn))
			if !slices.Contains(notYetHelpers, declName(fn)) {
				t.Errorf("%s declares %s: make it an *App method in an app file, "+
					"or move it to a package of its own concern", name, declName(fn))
			}
		}
	}
	for _, name := range notYetHelpers {
		if !slices.Contains(found, name) {
			t.Errorf("%s is no longer a helper: take it off notYetHelpers", name)
		}
	}
}

// notYetShared are the *App methods a command file declares that other files
// call.
var notYetShared = []string{
	"App.claudePath", "App.gcKeep", "App.gcPick", "App.ghProvenance",
	"App.installedVersion", "App.remover", "App.sessionVersion",
}

// builder is the method that builds the command a file is named for, such as
// sessionMvCommand for session_mv.go. Its parent's file calls it.
func builder(name string) string {
	words := strings.Split(strings.TrimSuffix(name, ".go"), "_")
	for i := 1; i < len(words); i++ {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, "") + "Command"
}

func TestSharedAppMethodsLiveInAppFiles(t *testing.T) {
	files := parsed(t)
	// used is the names each file selects, as in app.name, or calls.
	used := map[string]map[string]bool{}
	for name, f := range files {
		used[name] = map[string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				used[name][n.Sel.Name] = true
			case *ast.CallExpr:
				if id, ok := n.Fun.(*ast.Ident); ok {
					used[name][id.Name] = true
				}
			}
			return true
		})
	}
	var found []string
	for name, f := range files {
		if isPlumbing(name) {
			continue
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !(fn.Recv != nil && isApp(fn.Recv.List[0].Type) || fn.Recv == nil && takesApp(fn)) ||
				fn.Name.Name == builder(name) {
				continue
			}
			var callers []string
			for other := range files {
				if other != name && used[other][fn.Name.Name] {
					callers = append(callers, other)
				}
			}
			if len(callers) == 0 {
				continue
			}
			found = append(found, declName(fn))
			if !slices.Contains(notYetShared, declName(fn)) {
				slices.Sort(callers)
				t.Errorf("%s declares %s, which %s also call: move it to an app file",
					name, declName(fn), strings.Join(callers, ", "))
			}
		}
	}
	for _, name := range notYetShared {
		if !slices.Contains(found, name) {
			t.Errorf("%s is no longer shared: take it off notYetShared", name)
		}
	}
}

func TestPassThrough(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{nil, []string{"claude", "agents"}},
		{[]string{"--cwd", ".", "--model", "opus"}, []string{"claude", "agents", "--cwd", ".", "--model", "opus"}},
		{[]string{"--"}, []string{"claude", "agents"}},
		{[]string{"--", "ls", "--json"}, []string{"claude", "agents", "ls", "--json"}},
		{[]string{"--", "--", "x"}, []string{"claude", "agents", "--", "x"}},
	} {
		h := newHarness(t)
		h.ok(t, tc.args...)
		if len(h.execs) != 1 || !slices.Equal(h.execs[0], tc.want) {
			t.Fatalf("%v: execs %q", tc.args, h.execs)
		}
	}
}

func TestOwnCommandsAndHelpDoNotPassThrough(t *testing.T) {
	for _, args := range [][]string{{"ls"}, {"list"}, {"--help"}, {"help"}, {"--version"}} {
		h := newHarness(t, nil)
		h.ok(t, args...)
		if len(h.execs) != 0 {
			t.Fatalf("%v passed through", args)
		}
	}
}

func TestAMissingHomeFolderStopsEveryCommand(t *testing.T) {
	for _, name := range []string{"HOME", "USERPROFILE", "CLAUDE_CONFIG_DIR", "XDG_STATE_HOME"} {
		t.Setenv(name, "")
	}
	app := newApp()
	var errOut bytes.Buffer
	app.Err = &errOut
	app.Agents = &fakeAgents{}
	if code := app.Run([]string{"restart"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	assertContains(t, errOut.String(), "cannot find")
}

func TestExitCodesSayWhatHappened(t *testing.T) {
	h := newHarness(t)
	one, two := h.bg("review-one", "idle"), h.bg("review-two", "idle")
	bad := h.bg("bad", "idle")
	h.agents.snapshots = [][]claude.Session{{one, two, bad}}

	h.exits(t, exit.NoMatch, "stop", "nothing-like-it")
	h.exits(t, exit.NoMatch, "stop", "review")

	h.agents.failures = map[string]string{"stop " + bad.JobID: "boom"}
	h.exits(t, exit.SomeFailed, "stop", "bad")

	odd := h.bg("odd", "idle")
	odd.Kind = "daemon"
	h.agents.snapshots = [][]claude.Session{{odd}}
	h.exits(t, exit.Refused, "stop", "odd")
}

func TestVerboseFirstStillRunsClaudesCommands(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	for _, flag := range []string{"-v", "--verbose", "--verbose=true"} {
		h.out.Reset()
		h.ok(t, flag, "ls")
		assertContains(t, h.out.String(), "api")
	}
	if len(h.execs) != 0 {
		t.Fatalf("passed through to claude agents: %q", h.execs)
	}
}

func TestPassThroughThatCannotRunClaude(t *testing.T) {
	h := newHarness(t)
	h.app.Exec = func(string, ...string) error { return errors.New("executable file not found") }

	h.fails(t, "--some-agents-flag")
	assertContains(t, h.err.String(), "claudes: cannot run claude agents: executable file not found")
}
