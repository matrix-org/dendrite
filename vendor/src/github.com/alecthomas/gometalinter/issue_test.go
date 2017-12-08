package main

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSortedIssues(t *testing.T) {
	actual := []*Issue{
		{Path: newIssuePath("", "b.go"), Line: 5, Col: 1},
		{Path: newIssuePath("", "a.go"), Line: 3, Col: 2},
		{Path: newIssuePath("", "b.go"), Line: 1, Col: 3},
		{Path: newIssuePath("", "a.go"), Line: 1, Col: 4},
	}
	issues := &sortedIssues{
		issues: actual,
		order:  []string{"path", "line", "column"},
	}
	sort.Sort(issues)
	expected := []*Issue{
		{Path: newIssuePath("", "a.go"), Line: 1, Col: 4},
		{Path: newIssuePath("", "a.go"), Line: 3, Col: 2},
		{Path: newIssuePath( "", "b.go"), Line: 1, Col: 3},
		{Path: newIssuePath( "", "b.go"), Line: 5, Col: 1},
	}
	require.Equal(t, expected, actual)
}

func TestCompareOrderWithMessage(t *testing.T) {
	order := []string{"path", "line", "column", "message"}
	issueM := Issue{Path: newIssuePath("", "file.go"), Message: "message"}
	issueU := Issue{Path: newIssuePath("", "file.go"), Message: "unknown"}

	assert.True(t, CompareIssue(issueM, issueU, order))
	assert.False(t, CompareIssue(issueU, issueM, order))
}
