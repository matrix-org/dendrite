package loghooks

import (
	"database/sql"

	"github.com/gchaincl/sqlhooks"
	sqlite3 "github.com/mattn/go-sqlite3"
)

func Example() {
	driver := sqlhooks.Wrap(&sqlite3.SQLiteDriver{}, New())
	sql.Register("sqlite3-logger", driver)
	db, _ := sql.Open("sqlite3-logger", ":memory:")

	// This query will output logs
	db.Query("SELECT 1+1")
}
