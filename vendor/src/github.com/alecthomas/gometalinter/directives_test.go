package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoreRangeMatch(t *testing.T) {
	var testcases = []struct {
		doc      string
		issue    Issue
		linters  []string
		expected bool
	}{
		{
			doc:   "unmatched line",
			issue: Issue{Line: 100},
		},
		{
			doc:      "matched line, all linters",
			issue:    Issue{Line: 5},
			expected: true,
		},
		{
			doc:     "matched line, unmatched linter",
			issue:   Issue{Line: 5},
			linters: []string{"vet"},
		},
		{
			doc:      "matched line and linters",
			issue:    Issue{Line: 20, Linter: "vet"},
			linters:  []string{"vet"},
			expected: true,
		},
	}

	for _, testcase := range testcases {
		ir := ignoredRange{col: 20, start: 5, end: 20, linters: testcase.linters}
		assert.Equal(t, testcase.expected, ir.matches(&testcase.issue), testcase.doc)
	}
}
