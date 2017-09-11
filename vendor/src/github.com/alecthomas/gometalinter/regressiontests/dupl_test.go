package regressiontests

import "testing"

func TestDupl(t *testing.T) {
	t.Parallel()
	source := `package test

func findVendoredLinters() string {
	gopaths := strings.Split(os.Getenv("GOPATH"), string(os.PathListSeparator))
	for _, home := range vendoredSearchPaths {
		for _, p := range gopaths {
			joined := append([]string{p, "src"}, home...)
			vendorRoot := filepath.Join(joined...)
			fmt.Println(vendorRoot)
			if _, err := os.Stat(vendorRoot); err == nil {
				return vendorRoot
			}
		}
	}
	return ""

}

func two() string {
	gopaths := strings.Split(os.Getenv("GOPATH"), string(os.PathListSeparator))
	for _, home := range vendoredSearchPaths {
		for _, p := range gopaths {
			joined := append([]string{p, "src"}, home...)
			vendorRoot := filepath.Join(joined...)
			fmt.Println(vendorRoot)
			if _, err := os.Stat(vendorRoot); err == nil {
				return vendorRoot
			}
		}
	}
	return ""

}
`

	expected := Issues{
		{Linter: "dupl", Severity: "warning", Path: "test.go", Line: 19, Col: 0, Message: "duplicate of test.go:3-17"},
		{Linter: "dupl", Severity: "warning", Path: "test.go", Line: 3, Col: 0, Message: "duplicate of test.go:19-33"},
	}
	ExpectIssues(t, "dupl", source, expected)
}
