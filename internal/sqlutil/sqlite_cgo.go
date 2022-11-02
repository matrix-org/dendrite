//go:build cgo
// +build cgo

package sqlutil

import (
	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
)

const SQLITE_DRIVER_NAME = "sqlite3"

func sqliteDSNExtension(dsn string) string {
	return dsn
}

func sqliteDriver() *sqlite3.SQLiteDriver {
	return &sqlite3.SQLiteDriver{}
}
