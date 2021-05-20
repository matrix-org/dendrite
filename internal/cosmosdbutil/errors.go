package cosmosdbutil

import (
	"database/sql"
	"errors"
)

// ErrNoRows is returned by Scan when QueryRow doesn't return a
// row. Used to simulate the SQL responses as its used for business logic
var ErrNoRows = sql.ErrNoRows

var ErrNotImplemented = errors.New("cosmosdb: not implemented")
