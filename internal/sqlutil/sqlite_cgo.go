//go:build cgo
// +build cgo

package sqlutil

import (
	_ "github.com/mattn/go-sqlite3"
)

const SQLITE_DRIVER_NAME = "sqlite3"

func sqliteDSNExtension(dsn string) string {
	return dsn
}
