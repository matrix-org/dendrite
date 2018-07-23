package sqlhooks

import (
	"database/sql"
	"os"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setUp(t *testing.T) func() {
	dbName := "sqlite3test.db"

	db, err := sql.Open("sqlite3", dbName)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec("CREATE table users(id int, name text)")
	require.NoError(t, err)

	return func() { os.Remove(dbName) }
}

func TestSQLite3(t *testing.T) {
	defer setUp(t)()
	s := newSuite(t, &sqlite3.SQLiteDriver{}, "sqlite3test.db")

	s.TestHooksExecution(t, "SELECT * FROM users WHERE id = ?", 1)
	s.TestHooksArguments(t, "SELECT * FROM users WHERE id = ? AND name = ?", int64(1), "Gus")
	s.TestHooksErrors(t, "SELECT 1+1")

	t.Run("DBWorks", func(t *testing.T) {
		s.hooks.noop()
		if _, err := s.db.Exec("DELETE FROM users"); err != nil {
			t.Fatal(err)
		}

		stmt, err := s.db.Prepare("INSERT INTO users (id, name) VALUES(?, ?)")
		require.NoError(t, err)
		for range [5]struct{}{} {
			_, err := stmt.Exec(time.Now().UnixNano(), "gus")
			require.NoError(t, err)
		}

		var count int
		require.NoError(t,
			s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count),
		)
		assert.Equal(t, 5, count)
	})
}
