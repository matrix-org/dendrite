// (c) Copyright 2016 Hewlett Packard Enterprise Development LP
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	gas "github.com/GoASTScanner/gas/core"
	"go/ast"
	"go/types"
)

type NoErrorCheck struct {
	gas.MetaData
	whitelist gas.CallList
}

func returnsError(callExpr *ast.CallExpr, ctx *gas.Context) int {
	if tv := ctx.Info.TypeOf(callExpr); tv != nil {
		switch t := tv.(type) {
		case *types.Tuple:
			for pos := 0; pos < t.Len(); pos += 1 {
				variable := t.At(pos)
				if variable != nil && variable.Type().String() == "error" {
					return pos
				}
			}
		case *types.Named:
			if t.String() == "error" {
				return 0
			}
		}
	}
	return -1
}

func (r *NoErrorCheck) Match(n ast.Node, ctx *gas.Context) (*gas.Issue, error) {
	switch stmt := n.(type) {
	case *ast.AssignStmt:
		for _, expr := range stmt.Rhs {
			if callExpr, ok := expr.(*ast.CallExpr); ok && !r.whitelist.ContainsCallExpr(callExpr, ctx) {
				pos := returnsError(callExpr, ctx)
				if pos < 0 || pos >= len(stmt.Lhs) {
					return nil, nil
				}
				if id, ok := stmt.Lhs[pos].(*ast.Ident); ok && id.Name == "_" {
					return gas.NewIssue(ctx, n, r.What, r.Severity, r.Confidence), nil
				}
			}
		}
	case *ast.ExprStmt:
		if callExpr, ok := stmt.X.(*ast.CallExpr); ok && !r.whitelist.ContainsCallExpr(callExpr, ctx) {
			pos := returnsError(callExpr, ctx)
			if pos >= 0 {
				return gas.NewIssue(ctx, n, r.What, r.Severity, r.Confidence), nil
			}
		}
	}
	return nil, nil
}

func NewNoErrorCheck(conf map[string]interface{}) (gas.Rule, []ast.Node) {

	// TODO(gm) Come up with sensible defaults here. Or flip it to use a
	// black list instead.
	whitelist := gas.NewCallList()
	whitelist.AddAll("bytes.Buffer", "Write", "WriteByte", "WriteRune", "WriteString")
	whitelist.AddAll("fmt", "Print", "Printf", "Println")
	whitelist.Add("io.PipeWriter", "CloseWithError")

	if configured, ok := conf["G104"]; ok {
		if whitelisted, ok := configured.(map[string][]string); ok {
			for key, val := range whitelisted {
				whitelist.AddAll(key, val...)
			}
		}
	}
	return &NoErrorCheck{
		MetaData: gas.MetaData{
			Severity:   gas.Low,
			Confidence: gas.High,
			What:       "Errors unhandled.",
		},
		whitelist: whitelist,
	}, []ast.Node{(*ast.AssignStmt)(nil), (*ast.ExprStmt)(nil)}
}
