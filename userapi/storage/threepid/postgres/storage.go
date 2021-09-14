package postgres

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/storage/threepid/shared"
)

type Database struct {
	*shared.Database
}

const threePidSessionsSchema = `
-- This sequence is used for automatic allocation of session_id.
-- CREATE SEQUENCE IF NOT EXISTS threepid_session_id_seq START 1;

-- Stores data about devices.
CREATE TABLE IF NOT EXISTS threepid_sessions (
    sid SERIAL PRIMARY KEY,
    client_secret VARCHAR(255),
    threepid TEXT ,
    token VARCHAR(255) ,
    next_link TEXT,
    validated_at_ts BIGINT,
    validated BOOLEAN,
    send_attempt INT
);

CREATE UNIQUE INDEX IF NOT EXISTS threepid_sessions_threepids
    ON threepid_sessions (threepid, client_secret)
`

func Open(dbProperties *config.DatabaseOptions) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}

	// Create the tables.
	if err = create(db); err != nil {
		return nil, err
	}
	var sharedDb *shared.Database
	sharedDb, err = shared.NewDatabase(db, sqlutil.NewDummyWriter())
	if err != nil {
		return nil, err
	}
	return &Database{
		Database: sharedDb,
	}, nil
}

func create(db *sql.DB) error {
	_, err := db.Exec(threePidSessionsSchema)
	return err
}
