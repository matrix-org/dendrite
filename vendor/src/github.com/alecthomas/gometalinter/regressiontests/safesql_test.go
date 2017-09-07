package regressiontests

import "testing"

func TestSafesql(t *testing.T) {
	t.Parallel()
	source := `package test

import (
	"database/sql"
	"log"
	"strconv"
)

func main() {
	getUser(42)
}

func getUser(userID int64) {
	db, err := sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/hello")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, name FROM users WHERE id=" + strconv.FormatInt(userID, 10))
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
}
`
	expected := Issues{
		{Linter: "safesql", Severity: "warning", Path: "test.go", Line: 20, Col: 23, Message: `potentially unsafe SQL statement`},
	}
	ExpectIssues(t, "safesql", source, expected)
}
