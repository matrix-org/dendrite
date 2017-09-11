package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"sync"
	"time"
)

type ignoredRange struct {
	col        int
	start, end int
	linters    []string
}

func (i *ignoredRange) matches(issue *Issue) bool {
	if issue.Line < i.start || issue.Line > i.end {
		return false
	}
	if len(i.linters) == 0 {
		return true
	}
	for _, l := range i.linters {
		if l == issue.Linter {
			return true
		}
	}
	return false
}

func (i *ignoredRange) near(col, start int) bool {
	return col == i.col && i.end == start-1
}

type ignoredRanges []*ignoredRange

func (ir ignoredRanges) Len() int           { return len(ir) }
func (ir ignoredRanges) Swap(i, j int)      { ir[i], ir[j] = ir[j], ir[i] }
func (ir ignoredRanges) Less(i, j int) bool { return ir[i].end < ir[j].end }

type directiveParser struct {
	lock  sync.Mutex
	files map[string]ignoredRanges
	fset  *token.FileSet
}

func newDirectiveParser() *directiveParser {
	return &directiveParser{
		files: map[string]ignoredRanges{},
		fset:  token.NewFileSet(),
	}
}

// IsIgnored returns true if the given linter issue is ignored by a linter directive.
func (d *directiveParser) IsIgnored(issue *Issue) bool {
	d.lock.Lock()
	ranges, ok := d.files[issue.Path]
	if !ok {
		ranges = d.parseFile(issue.Path)
		sort.Sort(ranges)
		d.files[issue.Path] = ranges
	}
	d.lock.Unlock()
	for _, r := range ranges {
		if r.matches(issue) {
			return true
		}
	}
	return false
}

// Takes a set of ignoredRanges, determines if they immediately precede a statement
// construct, and expands the range to include that construct. Why? So you can
// precede a function or struct with //nolint
type rangeExpander struct {
	fset   *token.FileSet
	ranges ignoredRanges
}

func (a *rangeExpander) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return a
	}
	startPos := a.fset.Position(node.Pos())
	start := startPos.Line
	end := a.fset.Position(node.End()).Line
	found := sort.Search(len(a.ranges), func(i int) bool {
		return a.ranges[i].end+1 >= start
	})
	if found < len(a.ranges) && a.ranges[found].near(startPos.Column, start) {
		r := a.ranges[found]
		if r.start > start {
			r.start = start
		}
		if r.end < end {
			r.end = end
		}
	}
	return a
}

func (d *directiveParser) parseFile(path string) ignoredRanges {
	start := time.Now()
	debug("nolint: parsing %s for directives", path)
	file, err := parser.ParseFile(d.fset, path, nil, parser.ParseComments)
	if err != nil {
		debug("nolint: failed to parse %q: %s", path, err)
		return nil
	}
	ranges := extractCommentGroupRange(d.fset, file.Comments...)
	visitor := &rangeExpander{fset: d.fset, ranges: ranges}
	ast.Walk(visitor, file)
	debug("nolint: parsing %s took %s", path, time.Since(start))
	return visitor.ranges
}

func extractCommentGroupRange(fset *token.FileSet, comments ...*ast.CommentGroup) (ranges ignoredRanges) {
	for _, g := range comments {
		for _, c := range g.List {
			text := strings.TrimLeft(c.Text, "/ ")
			var linters []string
			if strings.HasPrefix(text, "nolint") {
				if strings.HasPrefix(text, "nolint:") {
					for _, linter := range strings.Split(text[7:], ",") {
						linters = append(linters, strings.TrimSpace(linter))
					}
				}
				pos := fset.Position(g.Pos())
				rng := &ignoredRange{
					col:     pos.Column,
					start:   pos.Line,
					end:     fset.Position(g.End()).Line,
					linters: linters,
				}
				ranges = append(ranges, rng)
			}
		}
	}
	return
}

func filterIssuesViaDirectives(directives *directiveParser, issues chan *Issue) chan *Issue {
	out := make(chan *Issue, 1000000)
	go func() {
		for issue := range issues {
			if !directives.IsIgnored(issue) {
				out <- issue
			}
		}
		close(out)
	}()
	return out
}
