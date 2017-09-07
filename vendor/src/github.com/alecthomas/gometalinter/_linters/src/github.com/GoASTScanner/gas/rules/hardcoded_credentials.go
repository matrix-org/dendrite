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
	"go/token"
	"regexp"

	"github.com/nbutton23/zxcvbn-go"
	"strconv"
)

type Credentials struct {
	gas.MetaData
	pattern          *regexp.Regexp
	entropyThreshold float64
	perCharThreshold float64
	truncate         int
	ignoreEntropy    bool
}

func truncate(s string, n int) string {
	if n > len(s) {
		return s
	}
	return s[:n]
}

func (r *Credentials) isHighEntropyString(str string) bool {
	s := truncate(str, r.truncate)
	info := zxcvbn.PasswordStrength(s, []string{})
	entropyPerChar := info.Entropy / float64(len(s))
	return (info.Entropy >= r.entropyThreshold ||
		(info.Entropy >= (r.entropyThreshold/2) &&
			entropyPerChar >= r.perCharThreshold))
}

func (r *Credentials) Match(n ast.Node, ctx *gas.Context) (*gas.Issue, error) {
	switch node := n.(type) {
	case *ast.AssignStmt:
		return r.matchAssign(node, ctx)
	case *ast.GenDecl:
		return r.matchGenDecl(node, ctx)
	}
	return nil, nil
}

func (r *Credentials) matchAssign(assign *ast.AssignStmt, ctx *gas.Context) (*gas.Issue, error) {
	for _, i := range assign.Lhs {
		if ident, ok := i.(*ast.Ident); ok {
			if r.pattern.MatchString(ident.Name) {
				for _, e := range assign.Rhs {
					if val, err := gas.GetString(e); err == nil {
						if r.ignoreEntropy || (!r.ignoreEntropy && r.isHighEntropyString(val)) {
							return gas.NewIssue(ctx, assign, r.What, r.Severity, r.Confidence), nil
						}
					}
				}
			}
		}
	}
	return nil, nil
}

func (r *Credentials) matchGenDecl(decl *ast.GenDecl, ctx *gas.Context) (*gas.Issue, error) {
	if decl.Tok != token.CONST && decl.Tok != token.VAR {
		return nil, nil
	}
	for _, spec := range decl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			for index, ident := range valueSpec.Names {
				if r.pattern.MatchString(ident.Name) && valueSpec.Values != nil {
					// const foo, bar = "same value"
					if len(valueSpec.Values) <= index {
						index = len(valueSpec.Values) - 1
					}
					if val, err := gas.GetString(valueSpec.Values[index]); err == nil {
						if r.ignoreEntropy || (!r.ignoreEntropy && r.isHighEntropyString(val)) {
							return gas.NewIssue(ctx, valueSpec, r.What, r.Severity, r.Confidence), nil
						}
					}
				}
			}
		}
	}
	return nil, nil
}

func NewHardcodedCredentials(conf map[string]interface{}) (gas.Rule, []ast.Node) {
	pattern := `(?i)passwd|pass|password|pwd|secret|token`
	entropyThreshold := 80.0
	perCharThreshold := 3.0
	ignoreEntropy := false
	var truncateString int = 16
	if val, ok := conf["G101"]; ok {
		conf := val.(map[string]string)
		if configPattern, ok := conf["pattern"]; ok {
			pattern = configPattern
		}
		if configIgnoreEntropy, ok := conf["ignore_entropy"]; ok {
			if parsedBool, err := strconv.ParseBool(configIgnoreEntropy); err == nil {
				ignoreEntropy = parsedBool
			}
		}
		if configEntropyThreshold, ok := conf["entropy_threshold"]; ok {
			if parsedNum, err := strconv.ParseFloat(configEntropyThreshold, 64); err == nil {
				entropyThreshold = parsedNum
			}
		}
		if configCharThreshold, ok := conf["per_char_threshold"]; ok {
			if parsedNum, err := strconv.ParseFloat(configCharThreshold, 64); err == nil {
				perCharThreshold = parsedNum
			}
		}
		if configTruncate, ok := conf["truncate"]; ok {
			if parsedInt, err := strconv.Atoi(configTruncate); err == nil {
				truncateString = parsedInt
			}
		}
	}

	return &Credentials{
		pattern:          regexp.MustCompile(pattern),
		entropyThreshold: entropyThreshold,
		perCharThreshold: perCharThreshold,
		ignoreEntropy:    ignoreEntropy,
		truncate:         truncateString,
		MetaData: gas.MetaData{
			What:       "Potential hardcoded credentials",
			Confidence: gas.Low,
			Severity:   gas.High,
		},
	}, []ast.Node{(*ast.AssignStmt)(nil), (*ast.GenDecl)(nil)}
}
