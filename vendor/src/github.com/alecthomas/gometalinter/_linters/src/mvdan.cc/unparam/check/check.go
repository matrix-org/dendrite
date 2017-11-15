// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

// Package check implements the unparam linter. Note that its API is not
// stable.
package check // import "mvdan.cc/unparam/check"

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"io"
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
	"mvdan.cc/lint"
)

func UnusedParams(tests, debug bool, args ...string) ([]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	c := &Checker{
		wd:    wd,
		tests: tests,
	}
	if debug {
		c.debugLog = os.Stderr
	}
	return c.lines(args...)
}

type Checker struct {
	lprog *loader.Program
	prog  *ssa.Program

	wd string

	tests    bool
	debugLog io.Writer

	cachedDeclCounts map[string]map[string]int
}

var (
	_ lint.Checker = (*Checker)(nil)
	_ lint.WithSSA = (*Checker)(nil)

	skipValue = new(ssa.Value)
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

func (c *Checker) debug(format string, a ...interface{}) {
	if c.debugLog != nil {
		fmt.Fprintf(c.debugLog, format, a...)
	}
}

func (c *Checker) Check() ([]lint.Issue, error) {
	c.cachedDeclCounts = make(map[string]map[string]int)
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
		c.debug("func %s\n", fn.String())
		if dummyImpl(fn.Blocks[0]) { // panic implementation
			c.debug("  skip - dummy implementation\n")
			continue
		}
		for _, edge := range cg.Nodes[fn].In {
			switch edge.Site.Common().Value.(type) {
			case *ssa.Function:
			default:
				// called via a parameter or field, type
				// is set in stone.
				c.debug("  skip - type is required via call\n")
				continue funcLoop
			}
		}
		if c.multipleImpls(info, fn) {
			c.debug("  skip - multiple implementations via build tags\n")
			continue
		}

		callers := cg.Nodes[fn].In
		results := fn.Signature.Results()
		// skip exported funcs, as well as those that are
		// entirely unused
		if !ast.IsExported(fn.Name()) && len(callers) > 0 {
		resLoop:
			for i := 0; i < results.Len(); i++ {
				for _, edge := range callers {
					val := edge.Site.Value()
					if val == nil { // e.g. go statement
						continue
					}
					for _, instr := range *val.Referrers() {
						extract, ok := instr.(*ssa.Extract)
						if !ok {
							continue resLoop // direct, real use
						}
						if extract.Index != i {
							continue // not the same result param
						}
						if len(*extract.Referrers()) > 0 {
							continue resLoop // real use after extraction
						}
					}
				}
				res := results.At(i)
				name := paramDesc(i, res)
				issues = append(issues, Issue{
					pos: res.Pos(),
					msg: fmt.Sprintf("result %s is never used", name),
				})
			}
		}

		seen := make([]constant.Value, results.Len())
		numRets := 0
		for _, block := range fn.Blocks {
			last := block.Instrs[len(block.Instrs)-1]
			ret, ok := last.(*ssa.Return)
			if !ok {
				continue
			}
			for i, val := range ret.Results {
				cnst, ok := val.(*ssa.Const)
				switch {
				case !ok:
					seen[i] = nil
				case numRets == 0:
					seen[i] = cnst.Value
				case seen[i] == nil:
				case !constant.Compare(seen[i], token.EQL, cnst.Value):
					seen[i] = nil
				}
			}
			numRets++
		}
		if numRets > 1 {
			for i, val := range seen {
				if val == nil {
					continue
				}
				res := results.At(i)
				name := paramDesc(i, res)
				issues = append(issues, Issue{
					pos: res.Pos(),
					msg: fmt.Sprintf("result %s is always %s", name, val.String()),
				})
			}
		}

		for i, par := range fn.Params {
			if i == 0 && fn.Signature.Recv() != nil { // receiver
				continue
			}
			c.debug("%s\n", par.String())
			switch par.Object().Name() {
			case "", "_": // unnamed
				c.debug("  skip - unnamed\n")
				continue
			}
			reason := "is unused"
			if cv := receivesSameValue(cg.Nodes[fn].In, par, i); cv != nil {
				reason = fmt.Sprintf("always receives %v", cv)
			} else if anyRealUse(par, i) {
				c.debug("  skip - used somewhere in the func body\n")
				continue
			}
			issues = append(issues, Issue{
				pos: par.Pos(),
				msg: fmt.Sprintf("%s %s", par.Name(), reason),
			})
		}

	}
	// TODO: replace by sort.Slice once we drop Go 1.7 support
	sort.Sort(byNamePos{c.prog.Fset, issues})
	return issues, nil
}

type byNamePos struct {
	fset *token.FileSet
	l    []lint.Issue
}

func (p byNamePos) Len() int      { return len(p.l) }
func (p byNamePos) Swap(i, j int) { p.l[i], p.l[j] = p.l[j], p.l[i] }
func (p byNamePos) Less(i, j int) bool {
	p1 := p.fset.Position(p.l[i].Pos())
	p2 := p.fset.Position(p.l[j].Pos())
	if p1.Filename == p2.Filename {
		return p1.Offset < p2.Offset
	}
	return p1.Filename < p2.Filename
}

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
		switch x := ref.(type) {
		case *ssa.Call:
			if x.Call.Value != par.Parent() {
				return true // not a recursive call
			}
			for i, arg := range x.Call.Args {
				if arg != par {
					continue
				}
				if i == pos {
					// reused directly in a recursive call
					continue refLoop
				}
			}
			return true
		case *ssa.Store:
			if insertedStore(x) {
				continue // inserted by go/ssa, not from the code
			}
			return true
		default:
			return true
		}
	}
	return false
}

func insertedStore(instr ssa.Instruction) bool {
	if instr.Pos() != token.NoPos {
		return false
	}
	store, ok := instr.(*ssa.Store)
	if !ok {
		return false
	}
	alloc, ok := store.Addr.(*ssa.Alloc)
	// we want exactly one use of this alloc value for it to be
	// inserted by ssa and dummy - the alloc instruction itself.
	return ok && len(*alloc.Referrers()) == 1
}

var rxHarmlessCall = regexp.MustCompile(`(?i)\b(log(ger)?|errors)\b|\bf?print`)

// dummyImpl reports whether a block is a dummy implementation. This is
// true if the block will almost immediately panic, throw or return
// constants only.
func dummyImpl(blk *ssa.BasicBlock) bool {
	var ops [8]*ssa.Value
	for _, instr := range blk.Instrs {
		if insertedStore(instr) {
			continue // inserted by go/ssa, not from the code
		}
		for _, val := range instr.Operands(ops[:0]) {
			switch x := (*val).(type) {
			case nil, *ssa.Const, *ssa.ChangeType, *ssa.Alloc,
				*ssa.MakeInterface, *ssa.Function,
				*ssa.Global, *ssa.IndexAddr, *ssa.Slice,
				*ssa.UnOp:
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

func paramDesc(i int, v *types.Var) string {
	name := v.Name()
	if name != "" {
		return name
	}
	return fmt.Sprintf("%d (%s)", i, v.Type().String())
}
