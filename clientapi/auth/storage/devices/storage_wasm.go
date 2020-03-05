package devices

import (
	"fmt"
	"net/url"

	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices/sqlite3"
	"github.com/matrix-org/gomatrixserverlib"
)

func NewDatabase(dataSourceName string, serverName gomatrixserverlib.ServerName) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("Cannot use postgres implementation")
	}
	switch uri.Scheme {
	case "postgres":
		return nil, fmt.Errorf("Cannot use postgres implementation")
	case "file":
		return sqlite3.NewDatabase(dataSourceName, serverName)
	default:
		return nil, fmt.Errorf("Cannot use postgres implementation")
	}
}
