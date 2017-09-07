// Command safesql is a tool for performing static analysis on programs to
// ensure that SQL injection attacks are not possible. It does this by ensuring
// package database/sql is only used with compile-time constant queries.
package main

import (
	"flag"
	"fmt"
	"go/build"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/pointer"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func main() {
	var verbose, quiet bool
	flag.BoolVar(&verbose, "v", false, "Verbose mode")
	flag.BoolVar(&quiet, "q", false, "Only print on failure")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-q] [-v] package1 [package2 ...]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()
	pkgs := flag.Args()
	if len(pkgs) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	c := loader.Config{
		FindPackage: FindPackage,
	}
	c.Import("database/sql")
	for _, pkg := range pkgs {
		c.Import(pkg)
	}
	p, err := c.Load()
	if err != nil {
		fmt.Printf("error loading packages %v: %v\n", pkgs, err)
		os.Exit(2)
	}
	s := ssautil.CreateProgram(p, 0)
	s.Build()

	qms := FindQueryMethods(p.Package("database/sql").Pkg, s)
	if verbose {
		fmt.Println("database/sql functions that accept queries:")
		for _, m := range qms {
			fmt.Printf("- %s (param %d)\n", m.Func, m.Param)
		}
		fmt.Println()
	}

	mains := FindMains(p, s)
	if len(mains) == 0 {
		fmt.Println("Did not find any commands (i.e., main functions).")
		os.Exit(2)
	}

	res, err := pointer.Analyze(&pointer.Config{
		Mains:          mains,
		BuildCallGraph: true,
	})
	if err != nil {
		fmt.Printf("error performing pointer analysis: %v\n", err)
		os.Exit(2)
	}

	bad := FindNonConstCalls(res.CallGraph, qms)
	if len(bad) == 0 {
		if !quiet {
			fmt.Println(`You're safe from SQL injection! Yay \o/`)
		}
		return
	}

	fmt.Printf("Found %d potentially unsafe SQL statements:\n", len(bad))
	for _, ci := range bad {
		pos := p.Fset.Position(ci.Pos())
		fmt.Printf("- %s\n", pos)
	}
	fmt.Println("Please ensure that all SQL queries you use are compile-time constants.")
	fmt.Println("You should always use parameterized queries or prepared statements")
	fmt.Println("instead of building queries from strings.")
	os.Exit(1)
}

// QueryMethod represents a method on a type which has a string parameter named
// "query".
type QueryMethod struct {
	Func     *types.Func
	SSA      *ssa.Function
	ArgCount int
	Param    int
}

// FindQueryMethods locates all methods in the given package (assumed to be
// package database/sql) with a string parameter named "query".
func FindQueryMethods(sql *types.Package, ssa *ssa.Program) []*QueryMethod {
	methods := make([]*QueryMethod, 0)
	scope := sql.Scope()
	for _, name := range scope.Names() {
		o := scope.Lookup(name)
		if !o.Exported() {
			continue
		}
		if _, ok := o.(*types.TypeName); !ok {
			continue
		}
		n := o.Type().(*types.Named)
		for i := 0; i < n.NumMethods(); i++ {
			m := n.Method(i)
			if !m.Exported() {
				continue
			}
			s := m.Type().(*types.Signature)
			if num, ok := FuncHasQuery(s); ok {
				methods = append(methods, &QueryMethod{
					Func:     m,
					SSA:      ssa.FuncValue(m),
					ArgCount: s.Params().Len(),
					Param:    num,
				})
			}
		}
	}
	return methods
}

var stringType types.Type = types.Typ[types.String]

// FuncHasQuery returns the offset of the string parameter named "query", or
// none if no such parameter exists.
func FuncHasQuery(s *types.Signature) (offset int, ok bool) {
	params := s.Params()
	for i := 0; i < params.Len(); i++ {
		v := params.At(i)
		if v.Name() == "query" && v.Type() == stringType {
			return i, true
		}
	}
	return 0, false
}

// FindMains returns the set of all packages loaded into the given
// loader.Program which contain main functions
func FindMains(p *loader.Program, s *ssa.Program) []*ssa.Package {
	ips := p.InitialPackages()
	mains := make([]*ssa.Package, 0, len(ips))
	for _, info := range ips {
		ssaPkg := s.Package(info.Pkg)
		if ssaPkg.Func("main") != nil {
			mains = append(mains, ssaPkg)
		}
	}
	return mains
}

// FindNonConstCalls returns the set of callsites of the given set of methods
// for which the "query" parameter is not a compile-time constant.
func FindNonConstCalls(cg *callgraph.Graph, qms []*QueryMethod) []ssa.CallInstruction {
	cg.DeleteSyntheticNodes()

	// package database/sql has a couple helper functions which are thin
	// wrappers around other sensitive functions. Instead of handling the
	// general case by tracing down callsites of wrapper functions
	// recursively, let's just whitelist the functions we're already
	// tracking, since it happens to be good enough for our use case.
	okFuncs := make(map[*ssa.Function]struct{}, len(qms))
	for _, m := range qms {
		okFuncs[m.SSA] = struct{}{}
	}

	bad := make([]ssa.CallInstruction, 0)
	for _, m := range qms {
		node := cg.CreateNode(m.SSA)
		for _, edge := range node.In {
			if _, ok := okFuncs[edge.Site.Parent()]; ok {
				continue
			}
			cc := edge.Site.Common()
			args := cc.Args
			// The first parameter is occasionally the receiver.
			if len(args) == m.ArgCount+1 {
				args = args[1:]
			} else if len(args) != m.ArgCount {
				panic("arg count mismatch")
			}
			v := args[m.Param]
			if _, ok := v.(*ssa.Const); !ok {
				bad = append(bad, edge.Site)
			}
		}
	}

	return bad
}

// Deal with GO15VENDOREXPERIMENT
func FindPackage(ctxt *build.Context, path, dir string, mode build.ImportMode) (*build.Package, error) {
	if !useVendor {
		return ctxt.Import(path, dir, mode)
	}

	// First, walk up the filesystem from dir looking for vendor directories
	var vendorDir string
	for tmp := dir; vendorDir == "" && tmp != "/"; tmp = filepath.Dir(tmp) {
		dname := filepath.Join(tmp, "vendor", filepath.FromSlash(path))
		fd, err := os.Open(dname)
		if err != nil {
			continue
		}
		// Directories are only valid if they contain at least one file
		// with suffix ".go" (this also ensures that the file descriptor
		// we have is in fact a directory)
		names, err := fd.Readdirnames(-1)
		if err != nil {
			continue
		}
		for _, name := range names {
			if strings.HasSuffix(name, ".go") {
				vendorDir = filepath.ToSlash(dname)
				break
			}
		}
	}

	if vendorDir != "" {
		pkg, err := ctxt.ImportDir(vendorDir, mode)
		if err != nil {
			return nil, err
		}
		// Go tries to derive a valid import path for the package, but
		// it's wrong (it includes "/vendor/"). Overwrite it here.
		pkg.ImportPath = path
		return pkg, nil
	}

	return ctxt.Import(path, dir, mode)
}
