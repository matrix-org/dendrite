package sqlutil

import (
	"context"
	"database/sql"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestShouldReturnCorrectAmountOfResulstIfFewerVariablesThanLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	limit := uint(4)

	r := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3)

	mock.ExpectQuery("SELECT id WHERE id IN \\((\\$[0-9]{1,4},?\\s?){3}\\)").WillReturnRows(r)
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assert.NoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	}, iKeyIDs, limit)
	assert.NoError(t, err, "Call returned an error")
	assert.Len(t, result, len(v), "Result should be 3 long")
}

func TestShouldReturnCorrectAmountOfResulstIfEqualVariablesAsLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	limit := uint(4)

	r := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3).
		AddRow(4)

	mock.ExpectQuery("SELECT id WHERE id IN \\((\\$[0-9]{1,4},?\\s?){4}\\)").WillReturnRows(r)
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3, 4}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assert.NoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	}, iKeyIDs, limit)
	assert.NoError(t, err, "Call returned an error")
	assert.Len(t, result, len(v), "Result should be 3 long")
}

func TestShouldReturnCorrectAmountOfResultsIfMoreVariablesThanLimit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	limit := uint(4)

	r1 := mock.NewRows([]string{"id"}).
		AddRow(1).
		AddRow(2).
		AddRow(3).
		AddRow(4)

	r2 := mock.NewRows([]string{"id"}).
		AddRow(5)

	mock.ExpectQuery("SELECT id WHERE id IN \\((\\$[0-9]{1,4},?\\s?){4}\\)").WillReturnRows(r1)
	mock.ExpectQuery("SELECT id WHERE id IN \\((\\$[0-9]{1,4},?\\s?){1}\\)").WillReturnRows(r2)
	q := "SELECT id WHERE id IN ($1)"
	v := []int{1, 2, 3, 4, 5}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]int, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, func(rows *sql.Rows) error {
		for rows.Next() {
			var id int
			err = rows.Scan(&id)
			assert.NoError(t, err, "rows.Scan returned an error")
			result = append(result, id)
		}
		return nil
	}, iKeyIDs, limit)
	assert.NoError(t, err, "Call returned an error")
	assert.Equal(t, v, result, "Result is not as expected")
	assert.Len(t, result, len(v), "Result should be 3 long")
}

func TestShouldREturnErrorIfRowsScanReturnsError(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	limit := uint(4)

	r := mock.NewRows([]string{"id"}).
		AddRow("hej").
		AddRow(2).
		AddRow(3)

	mock.ExpectQuery("SELECT id WHERE id IN \\((\\$[0-9]{1,4},?\\s?){3}\\)").WillReturnRows(r)
	q := "SELECT id WHERE id IN ($1)"
	v := []int{-1, -2, 3}
	iKeyIDs := make([]interface{}, len(v))
	for i, d := range v {
		iKeyIDs[i] = d
	}

	ctx := context.Background()
	var result = make([]uint, 0)
	err = RunLimitedVariablesQuery(ctx, q, db, func(rows *sql.Rows) error {
		for rows.Next() {
			var id uint
			err = rows.Scan(&id)
			if err != nil {
				return err
			}
			result = append(result, id)
		}
		return nil
	}, iKeyIDs, limit)
	assert.Error(t, err, "Call did not return an error")
}
