// +build !wasm

package accounts

import (
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts/postgres"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewDatabase(dataSourceName, serverName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewDatabase(dataSourceName, serverName)
	case "file":
		return sqlite3.NewDatabase(dataSourceName, serverName)
	default:
		return postgres.NewDatabase(dataSourceName, serverName)
	}
}
