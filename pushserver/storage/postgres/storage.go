package postgres

import (
	"database/sql"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/pushserver/storage/shared"
	"github.com/matrix-org/dendrite/setup/config"
)

type Database struct {
	shared.Database
	sqlutil.PartitionOffsetStatements
}

func Open(dbProperties *config.DatabaseOptions) (*Database, error) {
	var d Database
	var err error
	if d.DB, err = sqlutil.Open(dbProperties); err != nil {
		return nil, fmt.Errorf("sqlutil.Open: %w", err)
	}
	d.Writer = sqlutil.NewDummyWriter()

	if err = d.PartitionOffsetStatements.Prepare(d.DB, d.Writer, "pushserver"); err != nil {
		return nil, err
	}

	if err = createNotificationsTable(d.DB); err != nil {
		return nil, err
	}
	if err = shared.CreatePushersTable(d.DB); err != nil {
		return nil, err
	}
	if err = d.Database.Prepare(); err != nil {
		return nil, err
	}

	return &d, nil
}

func createNotificationsTable(db *sql.DB) error {
	_, err := db.Exec(notificationsSchema)
	return err
}

const notificationsSchema = `
CREATE TABLE IF NOT EXISTS pushserver_notifications (
    id BIGSERIAL PRIMARY KEY,
	localpart TEXT NOT NULL,
	room_id TEXT NOT NULL,
	event_id TEXT NOT NULL,
    ts_ms BIGINT NOT NULL,
    highlight BOOLEAN NOT NULL,
    notification_json TEXT NOT NULL,
    read BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS notification_localpart_room_id_event_id_idx ON pushserver_notifications(localpart, room_id, event_id);
CREATE INDEX IF NOT EXISTS notification_localpart_room_id_id_idx ON pushserver_notifications(localpart, room_id, id);
CREATE INDEX IF NOT EXISTS notification_localpart_id_idx ON pushserver_notifications(localpart, id);
`
