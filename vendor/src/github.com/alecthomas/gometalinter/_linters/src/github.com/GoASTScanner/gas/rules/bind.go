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
	"go/ast"
	"regexp"

	gas "github.com/GoASTScanner/gas/core"
)

// Looks for net.Listen("0.0.0.0") or net.Listen(":8080")
type BindsToAllNetworkInterfaces struct {
	gas.MetaData
	call    *regexp.Regexp
	pattern *regexp.Regexp
}

func (r *BindsToAllNetworkInterfaces) Match(n ast.Node, c *gas.Context) (gi *gas.Issue, err error) {
	if node := gas.MatchCall(n, r.call); node != nil {
		if arg, err := gas.GetString(node.Args[1]); err == nil {
			if r.pattern.MatchString(arg) {
				return gas.NewIssue(c, n, r.What, r.Severity, r.Confidence), nil
			}
		}
	}
	return
}

func NewBindsToAllNetworkInterfaces(conf map[string]interface{}) (gas.Rule, []ast.Node) {
	return &BindsToAllNetworkInterfaces{
		call:    regexp.MustCompile(`^(net|tls)\.Listen$`),
		pattern: regexp.MustCompile(`^(0.0.0.0|:).*$`),
		MetaData: gas.MetaData{
			Severity:   gas.Medium,
			Confidence: gas.High,
			What:       "Binds to all network interfaces",
		},
	}, []ast.Node{(*ast.CallExpr)(nil)}
}
