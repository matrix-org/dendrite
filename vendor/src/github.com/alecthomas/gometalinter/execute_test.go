package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLinterStateCommand(t *testing.T) {
	varsDefault := Vars{"tests": "", "not_tests": "true"}
	varsWithTest := Vars{"tests": "true", "not_tests": ""}

	var testcases = []struct {
		linter   string
		vars     Vars
		expected string
	}{
		{
			linter:   "errcheck",
			vars:     varsWithTest,
			expected: `errcheck -abspath `,
		},
		{
			linter:   "errcheck",
			vars:     varsDefault,
			expected: `errcheck -abspath -ignoretests`,
		},
		{
			linter:   "gotype",
			vars:     varsDefault,
			expected: `gotype -e `,
		},
		{
			linter:   "gotype",
			vars:     varsWithTest,
			expected: `gotype -e -t`,
		},
		{
			linter:   "structcheck",
			vars:     varsDefault,
			expected: `structcheck `,
		},
		{
			linter:   "structcheck",
			vars:     varsWithTest,
			expected: `structcheck -t`,
		},
		{
			linter: "unparam",
			vars: varsDefault,
			expected: `unparam -tests=false`,
		},
		{
			linter: "unparam",
			vars: varsWithTest,
			expected: `unparam `,
		},
	}

	for _, testcase := range testcases {
		ls := linterState{
			Linter: getLinterByName(testcase.linter, LinterConfig{}),
			vars:   testcase.vars,
		}
		assert.Equal(t, testcase.expected, ls.command())
	}
}
