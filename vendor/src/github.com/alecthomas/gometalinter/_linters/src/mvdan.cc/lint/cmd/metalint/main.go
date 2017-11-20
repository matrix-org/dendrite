// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main // import "mvdan.cc/lint/cmd/metalint"

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/loader"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"mvdan.cc/lint"

	"github.com/kisielk/gotool"

	interfacer "mvdan.cc/interfacer/check"
	unparam "mvdan.cc/unparam/check"
)

var tests = flag.Bool("tests", false, "include tests")

func main() {
	flag.Parse()
	if err := runLinters(flag.Args()...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var linters = [...]struct {
	name    string
	checker lint.Checker
}{
	{"unparam", &unparam.Checker{}},
	{"interfacer", &interfacer.Checker{}},
}

type metaChecker struct {
	wd string

	lprog *loader.Program
	prog  *ssa.Program
}

func runLinters(args ...string) error {
	paths := gotool.ImportPaths(args)
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	c := &metaChecker{wd: wd}
	var conf loader.Config
	if _, err := conf.FromArgs(paths, *tests); err != nil {
		return err
	}
	if c.lprog, err = conf.Load(); err != nil {
		return err
	}
	for _, l := range linters {
		l.checker.Program(c.lprog)
		if ssaChecker, ok := l.checker.(lint.WithSSA); ok {
			if c.prog == nil {
				c.prog = ssautil.CreateProgram(c.lprog, 0)
				c.prog.Build()
			}
			ssaChecker.ProgramSSA(c.prog)
		}
		issues, err := l.checker.Check()
		if err != nil {
			return err
		}
		c.printIssues(l.name, issues)
	}
	return nil
}

func (c *metaChecker) printIssues(name string, issues []lint.Issue) {
	for _, issue := range issues {
		fpos := c.lprog.Fset.Position(issue.Pos()).String()
		if strings.HasPrefix(fpos, c.wd) {
			fpos = fpos[len(c.wd)+1:]
		}
		fmt.Printf("%s: %s (%s)\n", fpos, issue.Message(), name)
	}
}
