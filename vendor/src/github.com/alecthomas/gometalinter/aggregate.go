package main

import (
	"sort"
	"strings"
)

type (
	issueKey struct {
		path      string
		line, col int
		message   string
	}

	multiIssue struct {
		*Issue
		linterNames []string
	}
)

func maybeAggregateIssues(issues chan *Issue) chan *Issue {
	if !config.Aggregate {
		return issues
	}
	return aggregateIssues(issues)
}

func aggregateIssues(issues chan *Issue) chan *Issue {
	out := make(chan *Issue, 1000000)
	issueMap := make(map[issueKey]*multiIssue)
	go func() {
		for issue := range issues {
			key := issueKey{
				path:    issue.Path,
				line:    issue.Line,
				col:     issue.Col,
				message: issue.Message,
			}
			if existing, ok := issueMap[key]; ok {
				existing.linterNames = append(existing.linterNames, issue.Linter)
			} else {
				issueMap[key] = &multiIssue{
					Issue:       issue,
					linterNames: []string{issue.Linter},
				}
			}
		}
		for _, multi := range issueMap {
			issue := multi.Issue
			sort.Strings(multi.linterNames)
			issue.Linter = strings.Join(multi.linterNames, ", ")
			out <- issue
		}
		close(out)
	}()
	return out
}
