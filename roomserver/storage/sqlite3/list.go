package sqlite3

import (
	"strconv"
	"strings"

	"github.com/lib/pq"
)

type SqliteList string

func sqliteIn(a pq.Int64Array) string {
	var b []string
	for _, n := range a {
		b = append(b, strconv.FormatInt(n, 10))
	}
	return strings.Join(b, ",")
}

func sqliteInStr(a pq.StringArray) string {
	return "\"" + strings.Join(a, "\",\"") + "\""
}
