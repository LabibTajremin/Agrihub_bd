// Package archtest enforces the architecture laws (BUILD_INSTRUCTION §3) by
// statically inspecting every non-test Go file:
//
//	domain-purity    domain imports only stdlib and platform/errs
//	module-isolation a module never imports another module (except the
//	                 aiadapter port package, which is a published contract)
//	platform-pure    platform never imports modules or the app
//	no-sql-usecase   use cases never import database drivers or platform/database
//	transport-repo   transport never imports repository
//	no-time-now      time.Now only inside platform/clock
//	no-init          no init() functions
//	no-panic         no panic() in modules
package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Violation is one broken rule.
type Violation struct {
	Rule string
	File string
	Msg  string
}

func (v Violation) String() string { return v.Rule + ": " + v.File + ": " + v.Msg }

// Check scans root (a module directory) whose import path is modulePath.
func Check(root, modulePath string) ([]Violation, error) {
	var out []Violation
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && (d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p) // p is always under root
		vs, err := checkFile(p, filepath.ToSlash(rel), modulePath)
		out = append(out, vs...)
		return err
	})
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out, err
}

// moduleOf returns the module name for a path under internal/modules/<name>/.
func moduleOf(rel string) string {
	rest, ok := strings.CutPrefix(rel, "internal/modules/")
	if !ok {
		return ""
	}
	name, _, _ := strings.Cut(rest, "/")
	return name
}

func layerOf(rel string) string {
	parts := strings.Split(path.Dir(rel), "/")
	if len(parts) >= 4 && parts[0] == "internal" && parts[1] == "modules" {
		return parts[3]
	}
	return ""
}

func isStdlib(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

func checkFile(abs, rel, mod string) ([]Violation, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, abs, nil, 0)
	if err != nil {
		return nil, err
	}
	var out []Violation
	add := func(rule, msg string) { out = append(out, Violation{Rule: rule, File: rel, Msg: msg}) }

	own := moduleOf(rel)
	layer := layerOf(rel)
	inPlatform := strings.HasPrefix(rel, "internal/platform/")
	timeName := ""

	for _, spec := range f.Imports {
		imp, _ := strconv.Unquote(spec.Path.Value) // parser guarantees a valid literal
		if imp == "time" {
			timeName = "time"
			if spec.Name != nil {
				timeName = spec.Name.Name
			}
		}
		internalRel, isInternal := strings.CutPrefix(imp, mod+"/")
		if layer == "domain" && !isStdlib(imp) && imp != mod+"/internal/platform/errs" {
			add("domain-purity", "imports "+imp)
		}
		if isInternal {
			target := moduleOf(internalRel)
			if own != "" && target != "" && target != own && internalRel != "internal/modules/aiadapter" {
				add("module-isolation", "module "+own+" imports "+imp)
			}
			if inPlatform && (target != "" || strings.HasPrefix(internalRel, "internal/app")) {
				add("platform-pure", "imports "+imp)
			}
			if layer == "usecase" && strings.HasPrefix(internalRel, "internal/platform/database") {
				add("no-sql-usecase", "imports "+imp)
			}
			if layer == "transport" && strings.HasPrefix(internalRel, "internal/modules/"+own+"/repository") {
				add("transport-repo", "imports "+imp)
			}
		}
		if layer == "usecase" && strings.HasPrefix(imp, "github.com/jackc/") {
			add("no-sql-usecase", "imports "+imp)
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil && x.Name.Name == "init" {
				add("no-init", "declares init()")
			}
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "panic" && own != "" {
				add("no-panic", "calls panic at "+fset.Position(x.Pos()).String())
			}
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && timeName != "" && sel.Sel.Name == "Now" {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == timeName && !strings.HasPrefix(rel, "internal/platform/clock/") {
					add("no-time-now", "calls time.Now; inject clock.Clock")
				}
			}
		}
		return true
	})
	return out, nil
}
