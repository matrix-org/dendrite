package sqlutil

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestShouldReturnCorrectAmountOfResulstIfFewerVariablesThanLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assertNoError(t, err, "Failed to make DB")
	limit := uint(4)

	r := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3)

	mock.ExpectQuery(`SELECT id WHERE id IN \(\$1, \$2, \$3\)`).WillReturnRows(r)
	// nolint:goconst
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, iKeyIDs, limit, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assertNoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	})
	assertNoError(t, err, "Call returned an error")
	if len(result) != len(v) {
		t.Fatalf("Result should be 3 long")
	}
}

func TestShouldReturnCorrectAmountOfResulstIfEqualVariablesAsLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assertNoError(t, err, "Failed to make DB")
	limit := uint(4)

	r := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3).
		AddRow(4)

	mock.ExpectQuery(`SELECT id WHERE id IN \(\$1, \$2, \$3, \$4\)`).WillReturnRows(r)
	// nolint:goconst
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3, 4}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, iKeyIDs, limit, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assertNoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	})
	assertNoError(t, err, "Call returned an error")
	if len(result) != len(v) {
		t.Fatalf("Result should be 4 long")
	}
}

func TestShouldReturnCorrectAmountOfResultsIfMoreVariablesThanLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assertNoError(t, err, "Failed to make DB")
	limit := uint(4)

	r1 := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3).
		AddRow(4)

	r2 := mock.NewRows([]string{"id"}).
		AddRow(5)

	mock.ExpectQuery(`SELECT id WHERE id IN \(\$1, \$2, \$3, \$4\)`).WillReturnRows(r1)
	mock.ExpectQuery(`SELECT id WHERE id IN \(\$1\)`).WillReturnRows(r2)
	// nolint:goconst
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3, 4, 5}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, iKeyIDs, limit, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assertNoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	})
	assertNoError(t, err, "Call returned an error")
	if len(result) != len(v) {
		t.Fatalf("Result should be 5 long")
	}
	if !reflect.DeepEqual(v, result) {
		t.Fatalf("Result is not as expected: got %v want %v", v, result)
	}
}

func TestShouldReturnErrorIfRowsScanReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assertNoError(t, err, "Failed to make DB")
	limit := uint(4)

	// adding a string ID should result in rows.Scan returning an error
	r := mock.NewRows([]string{"id"}).
		AddRow("hej").
		AddRow(2).
		AddRow(3)

	mock.ExpectQuery(`SELECT id WHERE id IN \(\$1, \$2, \$3\)`).WillReturnRows(r)
	// nolint:goconst
	q := "SELECT id WHERE id IN ($1)"
	v := []int{-1, -2, 3}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]uint, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, iKeyIDs, limit, func(rows *sql.Rows) error {
		for rows.Next() {
			var id uint
			err = rows.Scan(&id)
			if err != nil {
				return err
			}
			result = append(result, id)
		}
		return nil
	})
	if err == nil {
		t.Fatalf("Call did not return an error")
	}
}

func TestRunLimitedVariablesExec(t *testing.T) {
	db, mock, err := sqlmock.New()
	assertNoError(t, err, "Failed to make DB")

	// Query and expect two queries to be executed
	mock.ExpectExec(`DELETE FROM WHERE id IN \(\$1\, \$2\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM WHERE id IN \(\$1\, \$2\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	variables := []interface{}{
		1, 2, 3, 4,
	}

	query := "DELETE FROM WHERE id IN ($1)"

	if err = RunLimitedVariablesExec(context.Background(), query, db, variables, 2); err != nil {
		t.Fatal(err)
	}

	// Query again, but only 3 parameters, still queries two times
	mock.ExpectExec(`DELETE FROM WHERE id IN \(\$1\, \$2\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM WHERE id IN \(\$1\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err = RunLimitedVariablesExec(context.Background(), query, db, variables[:3], 2); err != nil {
		t.Fatal(err)
	}

	// Query again, but only 2 parameters, queries only once
	mock.ExpectExec(`DELETE FROM WHERE id IN \(\$1\, \$2\)`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err = RunLimitedVariablesExec(context.Background(), query, db, variables[:2], 2); err != nil {
		t.Fatal(err)
	}

	// Test with invalid query (typo) should return an error
	mock.ExpectExec(`DELTE FROM`).
		WillReturnResult(sqlmock.NewResult(0, 0)).
		WillReturnError(errors.New("typo in query"))

	if err = RunLimitedVariablesExec(context.Background(), "DELTE FROM", db, variables[:2], 2); err == nil {
		t.Fatal("expected an error, but got none")
	}
}

func assertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatal(msg)
}
