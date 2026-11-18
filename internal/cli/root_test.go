package cli

import (
	"context"
	"fmt"
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
)

// inParentsFile are the commands whose code lives in another command's file,
// each with why.
var inParentsFile = map[string]string{
	"claudes unpin":        "pinCommand builds pin and unpin, which differ only in what they write",
	"claudes config path":  "a few lines, so it stays in config.go",
	"claudes config get":   "a few lines, so it stays in config.go",
	"claudes config set":   "a few lines, so it stays in config.go",
	"claudes mcp profiles": "a few lines, so it stays in mcp.go",
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

type panicky struct{ *fakeAgents }

func (p panicky) Respawn(context.Context, string) error { panic("boom") }

func TestAPanicFreesTheRunLock(t *testing.T) {
	h := newHarness(t)
	h.agents.snapshots = [][]claude.Session{{h.bg("api", "idle")}}
	h.app.Agents = panicky{h.agents}
	func() {
		defer func() { _ = recover() }()
		h.run("restart", "api")
	}()
	if _, err := os.Stat(h.app.LockPath); err == nil {
		t.Fatal("the run lock outlived a panic")
	}
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
		if code := h.run(args...); code != exitRefused {
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

// A claudes suspended mid-run, such as by a closed laptop lid, stops
// refreshing the run lock. The lock names its process, so it stays while that
// process runs, and goes once it has ended.
func TestTheRunLockOutlivesASuspendButNotItsOwner(t *testing.T) {
	h := newHarness(t)
	s := h.bg("api", "idle")
	h.agents.snapshots = [][]claude.Session{{s}}
	if err := os.MkdirAll(h.app.LockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.app.LockPath, "owner"), fmt.Sprintf("4242 %s\n", start.Format(time.RFC3339Nano)))
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(h.app.LockPath, old, old); err != nil {
		t.Fatal(err)
	}
	h.app.Alive = func(pid int, started time.Time) bool { return pid == 4242 && started.Equal(start) }

	h.exits(t, exitBusy, "stop", "api")
	assertCalls(t, h)

	h.app.Alive = func(int, time.Time) bool { return false }
	h.ok(t, "stop", "api")
	assertCalls(t, h, "stop "+s.JobID)
}

func TestTheRunLockNamesItsProcess(t *testing.T) {
	h := newHarness(t)
	owner := h.app.lockOwner()
	h.app.Alive = func(pid int, started time.Time) bool { return pid == os.Getpid() && started.Equal(start) }
	if h.app.lockOwnerGone(owner) {
		t.Fatalf("owner %q of this process reads as gone", owner)
	}
	for _, bad := range []string{"", "x", "0 " + start.Format(time.RFC3339Nano), "42 yesterday"} {
		if !h.app.lockOwnerGone([]byte(bad)) {
			t.Errorf("owner %q reads as running", bad)
		}
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
	"cells.go", "cells_test.go", "cli_test.go", "colors_test.go", "compat.go",
	"compat_test.go", "completion.go", "completion_test.go",
	"config_edit_unix.go", "config_edit_windows.go", "dirs.go", "errors.go",
	"failures_test.go", "flags.go", "flags_test.go", "lifecycle_test.go",
	"log.go", "output.go", "output_test.go", "plan.go", "relaunch.go",
	"report.go", "report_test.go", "selection.go", "selection_test.go",
	"signals_unix.go", "signals_unix_test.go", "signals_windows.go", "version.go",
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

// notYetHelpers are the functions this package declares that belong in an app
// file as *App methods, or in a package of their own concern.
var notYetHelpers = []string{
	"activity", "age", "ago", "branchCell", "buildVersion", "checkFail",
	"checkOK", "checkWarn", "clearStyle", "completeKeys", "count", "countCell",
	"dash", "describeForCompletion", "dirCell", "dirGone", "doing", "duRow",
	"duTally", "duTop", "editFile", "entryOf", "exitCode", "firstLine",
	"firstPrompt", "gcCommandFor", "homeShort", "hostJob", "interruptContext",
	"isFinished", "joinNotes", "keepWhy", "kind", "labels", "labelsOf",
	"leftovers", "lifecycleFlags.selectOpts", "listMatch", "managedBy",
	"nameCell", "nameWithSource", "named", "noProfile", "notReached", "noteOf",
	"oneEach", "orList", "pickPast", "printCommands", "printManual",
	"printableList", "readTally", "realDir", "refOf", "refused", "relWithin",
	"removeCommand", "reports", "requireFinished", "requireRunning", "resolved",
	"runningIn", "sameDir", "sameName", "sidecarItem", "sizeDetail", "skipSet",
	"sleep", "stamp", "taskSummary", "tokenCount", "triageOf", "triageSize",
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
				_, excepted := plumbingFuncs[fn.Name.Name]
				fits = fn.Recv == nil && (takesApp(fn) || excepted)
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
	"App.acquire", "App.addForce", "App.checkBack", "App.claudePath",
	"App.compatible", "App.completePast", "App.completeResumable",
	"App.completeResumeSkip", "App.completeSessions", "App.completeSkip",
	"App.configPathErr", "App.countParked", "App.current", "App.dirIn",
	"App.dryRunDone", "App.filterPast", "App.gate", "App.gcKeep", "App.gcPick",
	"App.ghProvenance", "App.handOff", "App.installedVersion", "App.lastWords",
	"App.listedJob", "App.log", "App.midTurn", "App.onePast", "App.parkAndStop",
	"App.pastSessions", "App.printDirs", "App.printLeftOut", "App.printPlan",
	"App.printPlanOf", "App.printReport", "App.progress", "App.progressFailed",
	"App.protectedPins", "App.record", "App.refuse", "App.relaunch",
	"App.remover", "App.reportFailures", "App.resumeAs", "App.resumePast",
	"App.resumeSession", "App.rows", "App.sessionVersion", "App.setLogLevel",
	"App.splitHost", "App.startReport", "App.stopFailed", "App.stopHost",
	"App.stopSession", "App.teamNote", "App.unpark", "App.warnTeams",
	"App.writeTable", "listMatch", "notReached", "printManual",
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
