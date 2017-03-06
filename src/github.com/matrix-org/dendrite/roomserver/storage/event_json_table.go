package storage

import (
	"database/sql"
	"github.com/matrix-org/dendrite/roomserver/types"
)

const eventJSONSchema = `
-- Stores the JSON for each event. This kept separate from the main events
-- table to keep the rows in the main events table small.
CREATE TABLE IF NOT EXISTS event_json (
    -- Local numeric ID for the event.
    event_nid BIGINT NOT NULL PRIMARY KEY,
    -- The JSON for the event.
    -- Stored as TEXT because this should be valid UTF-8.
    -- Not stored as a JSONB because we always just pull the entire event
    -- so there is no point in postgres parsing it.
    -- Not stored as JSON because we already validate the JSON in the server
    -- so there is no point in postgres validating it.
    -- TODO: Should we be compressing the events with Snappy or DEFLATE?
    event_json TEXT NOT NULL
);
`

const insertEventJSONSQL = "" +
	"INSERT INTO event_json (event_nid, event_json) VALUES ($1, $2)" +
	" ON CONFLICT DO NOTHING"

// Bulk event JSON lookup by numeric event ID.
// Sort by the numeric event ID.
// This means that we can use binary search to lookup by numeric event ID.
const bulkSelectEventJSONSQL = "" +
	"SELECT event_nid, event_json FROM event_json" +
	" WHERE event_nid = ANY($1)" +
	" ORDER BY event_nid ASC"

type eventJSONStatements struct {
	insertEventJSONStmt     *sql.Stmt
	bulkSelectEventJSONStmt *sql.Stmt
}

func (s *eventJSONStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(eventJSONSchema)
	if err != nil {
		return
	}
	return statementList{
		{&s.insertEventJSONStmt, insertEventJSONSQL},
		{&s.bulkSelectEventJSONStmt, bulkSelectEventJSONSQL},
	}.prepare(db)
}

func (s *eventJSONStatements) insertEventJSON(eventNID types.EventNID, eventJSON []byte) error {
	_, err := s.insertEventJSONStmt.Exec(int64(eventNID), eventJSON)
	return err
}

type eventJSONPair struct {
	EventNID  types.EventNID
	EventJSON []byte
}

func (s *eventJSONStatements) bulkSelectEventJSON(eventNIDs []types.EventNID) ([]eventJSONPair, error) {
	rows, err := s.bulkSelectEventJSONStmt.Query(eventNIDsAsArray(eventNIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// We know that we will only get as many results as event NIDs
	// because of the unique constraint on event NIDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than NIDs so we adjust the length of the slice before returning it.
	results := make([]eventJSONPair, len(eventNIDs))
	i := 0
	for ; rows.Next(); i++ {
		result := &results[i]
		var eventNID int64
		if err := rows.Scan(&eventNID, &result.EventJSON); err != nil {
			return nil, err
		}
		result.EventNID = types.EventNID(eventNID)
	}
	return results[:i], nil
}
