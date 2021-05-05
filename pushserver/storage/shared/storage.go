package shared

import (
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
)

type Database struct {
	DB     *sql.DB
	Writer sqlutil.Writer
}
