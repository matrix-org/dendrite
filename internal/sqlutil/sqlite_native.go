//go:build !cgo
// +build !cgo

package sqlutil

import (
	"modernc.org/sqlite"
	"strings"
)

const SQLITE_DRIVER_NAME = "sqlite"

func sqliteDSNExtension(dsn string) string {
	// add query parameters to the dsn
	if strings.Contains(dsn, "?") {
		dsn += "&"
	} else {
		dsn += "?"
	}

	// wait some time before erroring if the db is locked
	// https://gitlab.com/cznic/sqlite/-/issues/106#note_1058094993
	dsn += "_pragma=busy_timeout%3d10000"
	return dsn
}

func sqliteDriver() *sqlite.Driver {
	return &sqlite.Driver{}
}
