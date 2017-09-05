// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

// Package check implements the unparam linter. Note that its API is not
// stable.
package check

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/kisielk/gotool"
	"github.com/mvdan/lint"
)

func UnusedParams(tests bool, args ...string) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	c := &Checker{
		wd: wd, tests: tests,
		cachedDeclCounts: make(map[string]map[string]int),
	}
	return c.lines(args...)
}

type Checker struct {
	lprog *loader.Program
	prog  *ssa.Program

	wd string

	tests bool

	cachedDeclCounts map[string]map[string]int
}

var (
	_ lint.Checker = (*Checker)(nil)
	_ lint.WithSSA = (*Checker)(nil)
)

func (c *Checker) lines(args ...string) ([]string, error) {
	paths := gotool.ImportPaths(args)
	var conf loader.Config
	if _, err := conf.FromArgs(paths, c.tests); err != nil {
		return nil, err
	}
	lprog, err := conf.Load()
	if err != nil {
		return nil, err
	}
	prog := ssautil.CreateProgram(lprog, 0)
	prog.Build()
	c.Program(lprog)
	c.ProgramSSA(prog)
	issues, err := c.Check()
	if err != nil {
		return nil, err
	}
	lines := make([]string, len(issues))
	for i, issue := range issues {
		fpos := prog.Fset.Position(issue.Pos()).String()
		if strings.HasPrefix(fpos, c.wd) {
			fpos = fpos[len(c.wd)+1:]
		}
		lines[i] = fmt.Sprintf("%s: %s", fpos, issue.Message())
	}
	return lines, nil
}

type Issue struct {
	pos token.Pos
	msg string
}

func (i Issue) Pos() token.Pos  { return i.pos }
func (i Issue) Message() string { return i.msg }

func (c *Checker) Program(lprog *loader.Program) {
	c.lprog = lprog
}

func (c *Checker) ProgramSSA(prog *ssa.Program) {
	c.prog = prog
}

func (c *Checker) Check() ([]lint.Issue, error) {
	wantPkg := make(map[*types.Package]*loader.PackageInfo)
	for _, info := range c.lprog.InitialPackages() {
		wantPkg[info.Pkg] = info
	}
	cg := cha.CallGraph(c.prog)

	var issues []lint.Issue
funcLoop:
	for fn := range ssautil.AllFunctions(c.prog) {
		if fn.Pkg == nil { // builtin?
			continue
		}
		if len(fn.Blocks) == 0 { // stub
			continue
		}
		info := wantPkg[fn.Pkg.Pkg]
		if info == nil { // not part of given pkgs
			continue
		}
		if dummyImpl(fn.Blocks[0]) { // panic implementation
			continue
		}
		for _, edge := range cg.Nodes[fn].In {
			switch edge.Site.Common().Value.(type) {
			case *ssa.Function:
			default:
				// called via a parameter or field, type
				// is set in stone.
				continue funcLoop
			}
		}
		if c.multipleImpls(info, fn) {
			continue
		}
		for i, par := range fn.Params {
			if i == 0 && fn.Signature.Recv() != nil { // receiver
				continue
			}
			switch par.Object().Name() {
			case "", "_": // unnamed
				continue
			}
			reason := "is unused"
			if cv := receivesSameValue(cg.Nodes[fn].In, par, i); cv != nil {
				reason = fmt.Sprintf("always receives %v", cv)
			} else if anyRealUse(par, i) {
				continue
			}
			issues = append(issues, Issue{
				pos: par.Pos(),
				msg: fmt.Sprintf("%s %s", par.Name(), reason),
			})
		}

	}
	// TODO: replace by sort.Slice once we drop Go 1.7 support
	sort.Sort(byPos(issues))
	return issues, nil
}

type byPos []lint.Issue

func (p byPos) Len() int           { return len(p) }
func (p byPos) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p byPos) Less(i, j int) bool { return p[i].Pos() < p[j].Pos() }

func receivesSameValue(in []*callgraph.Edge, par *ssa.Parameter, pos int) constant.Value {
	if ast.IsExported(par.Parent().Name()) {
		// we might not have all call sites for an exported func
		return nil
	}
	var seen constant.Value
	for _, edge := range in {
		call := edge.Site.Common()
		cnst, ok := call.Args[pos].(*ssa.Const)
		if !ok {
			return nil // not a constant
		}
		if seen == nil {
			seen = cnst.Value // first constant
		} else if !constant.Compare(seen, token.EQL, cnst.Value) {
			return nil // different constants
		}
	}
	return seen
}

func anyRealUse(par *ssa.Parameter, pos int) bool {
refLoop:
	for _, ref := range *par.Referrers() {
		call, ok := ref.(*ssa.Call)
		if !ok {
			return true
		}
		if call.Call.Value != par.Parent() {
			return true // not a recursive call
		}
		for i, arg := range call.Call.Args {
			if arg != par {
				continue
			}
			if i == pos {
				// reused directly in a recursive call
				continue refLoop
			}
		}
		return true
	}
	return false
}

var rxHarmlessCall = regexp.MustCompile(`(?i)\b(log(ger)?|errors)\b|\bf?print`)

// dummyImpl reports whether a block is a dummy implementation. This is
// true if the block will almost immediately panic, throw or return
// constants only.
func dummyImpl(blk *ssa.BasicBlock) bool {
	var ops [8]*ssa.Value
	for _, instr := range blk.Instrs {
		for _, val := range instr.Operands(ops[:0]) {
			switch x := (*val).(type) {
			case nil, *ssa.Const, *ssa.ChangeType, *ssa.Alloc,
				*ssa.MakeInterface, *ssa.Function,
				*ssa.Global, *ssa.IndexAddr, *ssa.Slice:
			case *ssa.Call:
				if rxHarmlessCall.MatchString(x.Call.Value.String()) {
					continue
				}
			default:
				return false
			}
		}
		switch x := instr.(type) {
		case *ssa.Alloc, *ssa.Store, *ssa.UnOp, *ssa.BinOp,
			*ssa.MakeInterface, *ssa.MakeMap, *ssa.Extract,
			*ssa.IndexAddr, *ssa.FieldAddr, *ssa.Slice,
			*ssa.Lookup, *ssa.ChangeType, *ssa.TypeAssert,
			*ssa.Convert, *ssa.ChangeInterface:
			// non-trivial expressions in panic/log/print
			// calls
		case *ssa.Return, *ssa.Panic:
			return true
		case *ssa.Call:
			if rxHarmlessCall.MatchString(x.Call.Value.String()) {
				continue
			}
			return x.Call.Value.Name() == "throw" // runtime's panic
		default:
			return false
		}
	}
	return false
}

func (c *Checker) declCounts(pkgDir string, pkgName string) map[string]int {
	if m := c.cachedDeclCounts[pkgDir]; m != nil {
		return m
	}
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, pkgDir, nil, 0)
	if err != nil {
		panic(err.Error())
		return nil
	}
	pkg := pkgs[pkgName]
	count := make(map[string]int)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			fd, _ := decl.(*ast.FuncDecl)
			if fd == nil {
				continue
			}
			name := astPrefix(fd.Recv) + fd.Name.Name
			count[name]++
		}
	}
	c.cachedDeclCounts[pkgDir] = count
	return count
}

func astPrefix(recv *ast.FieldList) string {
	if recv == nil {
		return ""
	}
	expr := recv.List[0].Type
	for {
		star, _ := expr.(*ast.StarExpr)
		if star == nil {
			break
		}
		expr = star.X
	}
	id := expr.(*ast.Ident)
	return id.Name + "."
}

func (c *Checker) multipleImpls(info *loader.PackageInfo, fn *ssa.Function) bool {
	if fn.Parent() != nil { // nested func
		return false
	}
	path := c.prog.Fset.Position(fn.Pos()).Filename
	if path == "" { // generated func, like init
		return false
	}
	count := c.declCounts(filepath.Dir(path), info.Pkg.Name())
	name := fn.Name()
	if fn.Signature.Recv() != nil {
		tp := fn.Params[0].Type()
		for {
			point, _ := tp.(*types.Pointer)
			if point == nil {
				break
			}
			tp = point.Elem()
		}
		named := tp.(*types.Named)
		name = named.Obj().Name() + "." + name
	}
	return count[name] > 1
}
