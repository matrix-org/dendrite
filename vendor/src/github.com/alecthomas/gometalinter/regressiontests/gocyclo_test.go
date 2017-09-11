package regressiontests

import "testing"

func TestGocyclo(t *testing.T) {
	t.Parallel()
	source := `package test

func processOutput(state *linterState, out []byte) {
	re := state.Match()
	all := re.FindAllSubmatchIndex(out, -1)
	debug("%s hits %d: %s", state.name, len(all), state.pattern)
	for _, indices := range all {
		group := [][]byte{}
		for i := 0; i < len(indices); i += 2 {
			fragment := out[indices[i]:indices[i+1]]
			group = append(group, fragment)
		}

		issue := &Issue{}
		issue.Linter = Linter(state.name)
		for i, name := range re.SubexpNames() {
			part := string(group[i])
			if name != "" {
				state.vars[name] = part
			}
			switch name {
			case "path":
				issue.Path = part

			case "line":
				n, err := strconv.ParseInt(part, 10, 32)
				kingpin.FatalIfError(err, "line matched invalid integer")
				issue.Line = int(n)

			case "col":
				n, err := strconv.ParseInt(part, 10, 32)
				kingpin.FatalIfError(err, "col matched invalid integer")
				issue.Col = int(n)

			case "message":
				issue.Message = part

			case "":
			}
		}
		if m, ok := linterMessageOverrideFlag[state.name]; ok {
			issue.Message = state.vars.Replace(m)
		}
		if sev, ok := linterSeverityFlag[state.name]; ok {
			issue.Severity = Severity(sev)
		} else {
			issue.Severity = "error"
		}
		if state.filter != nil && state.filter.MatchString(issue.String()) {
			continue
		}
		state.issues <- issue
	}
	return
}
`
	expected := Issues{
		{Linter: "gocyclo", Severity: "warning", Path: "test.go", Line: 3, Col: 0, Message: "cyclomatic complexity 14 of function processOutput() is high (> 10)"},
	}
	ExpectIssues(t, "gocyclo", source, expected)
}
