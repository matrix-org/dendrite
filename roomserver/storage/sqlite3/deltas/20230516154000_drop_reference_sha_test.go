package deltas

import (
	"testing"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/test"
	"github.com/element-hq/dendrite/test/testrig"
	"github.com/stretchr/testify/assert"
)

func TestUpDropEventReferenceSHAPrevEvents(t *testing.T) {

	cfg, ctx, close := testrig.CreateConfig(t, test.DBTypeSQLite)
	defer close()

	db, err := sqlutil.Open(&cfg.RoomServer.Database, sqlutil.NewExclusiveWriter())
	assert.Nil(t, err)
	assert.NotNil(t, db)
	defer db.Close()

	// create the table in the old layout
	_, err = db.ExecContext(ctx.Context(), `
 CREATE TABLE IF NOT EXISTS roomserver_previous_events (
    previous_event_id TEXT NOT NULL,
    previous_reference_sha256 BLOB,
    event_nids TEXT NOT NULL,
    UNIQUE (previous_event_id, previous_reference_sha256)
  );`)
	assert.Nil(t, err)

	// create the events table as well, slimmed down with one eventNID
	_, err = db.ExecContext(ctx.Context(), `
  CREATE TABLE IF NOT EXISTS roomserver_events (
    event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
    room_nid INTEGER NOT NULL
);

INSERT INTO roomserver_events (event_nid, room_nid) VALUES (1, 1)
`)
	assert.Nil(t, err)

	// insert duplicate prev events with different event_nids
	stmt, err := db.PrepareContext(ctx.Context(), `INSERT INTO roomserver_previous_events (previous_event_id, event_nids, previous_reference_sha256) VALUES ($1, $2, $3)`)
	assert.Nil(t, err)
	assert.NotNil(t, stmt)
	_, err = stmt.ExecContext(ctx.Context(), "1", "{1,2}", "a")
	assert.Nil(t, err)
	_, err = stmt.ExecContext(ctx.Context(), "1", "{1,2,3}", "b")
	assert.Nil(t, err)

	// execute the migration
	txn, err := db.Begin()
	assert.Nil(t, err)
	assert.NotNil(t, txn)
	err = UpDropEventReferenceSHAPrevEvents(ctx.Context(), txn)
	defer txn.Rollback()
	assert.NoError(t, err)
}
