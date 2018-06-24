// Copyright 2018 New Vector Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const appserviceEventsSchema = `
-- Stores events to be sent to application services
CREATE TABLE IF NOT EXISTS appservice_events (
	-- An auto-incrementing id unique to each event in the table
	id BIGSERIAL NOT NULL PRIMARY KEY,
	-- The ID of the application service the event will be sent to
	as_id TEXT NOT NULL,
	-- The ID of the event
	event_id TEXT NOT NULL,
	-- Unix seconds that the event was sent at from the originating server
	origin_server_ts BIGINT NOT NULL,
	-- The ID of the room that the event was sent in
	room_id TEXT NOT NULL,
	-- The type of the event (e.g. m.text)
	type TEXT NOT NULL,
	-- The ID of the user that sent the event
	sender TEXT NOT NULL,
	-- The JSON representation of the event's content. Text to avoid db JSON parsing
	event_content TEXT,
	-- The ID of the transaction that this event is a part of
	txn_id BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS appservice_events_as_id ON appservice_events(as_id);
`

const selectEventsByApplicationServiceIDSQL = "" +
	"SELECT id, event_id, origin_server_ts, room_id, type, sender, event_content, txn_id " +
	"FROM appservice_events WHERE as_id = $1 ORDER BY txn_id DESC, id ASC"

const countEventsByApplicationServiceIDSQL = "" +
	"SELECT COUNT(event_id) FROM appservice_events WHERE as_id = $1"

const insertEventSQL = "" +
	"INSERT INTO appservice_events(as_id, event_id, origin_server_ts, room_id, type, sender, event_content, txn_id) " +
	"VALUES ($1, $2, $3, $4, $5, $6, $7, $8)"

const updateTxnIDForEventsSQL = "" +
	"UPDATE appservice_events SET txn_id = $1 WHERE as_id = $2 AND id <= $3"

const deleteEventsBeforeAndIncludingIDSQL = "" +
	"DELETE FROM appservice_events WHERE as_id = $1 AND id <= $2"

type eventsStatements struct {
	selectEventsByApplicationServiceIDStmt *sql.Stmt
	countEventsByApplicationServiceIDStmt  *sql.Stmt
	insertEventStmt                        *sql.Stmt
	updateTxnIDForEventsStmt               *sql.Stmt
	deleteEventsBeforeAndIncludingIDStmt   *sql.Stmt
}

func (s *eventsStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(appserviceEventsSchema)
	if err != nil {
		return
	}

	if s.selectEventsByApplicationServiceIDStmt, err = db.Prepare(selectEventsByApplicationServiceIDSQL); err != nil {
		return
	}
	if s.countEventsByApplicationServiceIDStmt, err = db.Prepare(countEventsByApplicationServiceIDSQL); err != nil {
		return
	}
	if s.insertEventStmt, err = db.Prepare(insertEventSQL); err != nil {
		return
	}
	if s.updateTxnIDForEventsStmt, err = db.Prepare(updateTxnIDForEventsSQL); err != nil {
		return
	}
	if s.deleteEventsBeforeAndIncludingIDStmt, err = db.Prepare(deleteEventsBeforeAndIncludingIDSQL); err != nil {
		return
	}

	return
}

// selectEventsByApplicationServiceID takes in an application service ID and
// returns a slice of events that need to be sent to that application service,
// as well as an int later used to remove these same events from the database
// once successfully sent to an application service.
func (s *eventsStatements) selectEventsByApplicationServiceID(
	ctx context.Context,
	applicationServiceID string,
	limit int,
) (
	txnID, maxID int,
	events []gomatrixserverlib.ApplicationServiceEvent,
	err error,
) {
	// Retrieve events from the database. Unsuccessfully sent events first
	eventRowsCurr, err := s.selectEventsByApplicationServiceIDStmt.QueryContext(ctx, applicationServiceID)
	if err != nil {
		return 0, 0, nil, err
	}
	defer func() {
		err = eventRowsCurr.Close()
		if err != nil {
			log.WithError(err).Fatalf("Appservice %s unable to select new events to send",
				applicationServiceID)
		}
	}()
	events, maxID, txnID, err = retrieveEvents(eventRowsCurr, limit)
	if err != nil {
		return 0, 0, nil, err
	}

	return
}

func retrieveEvents(eventRows *sql.Rows, limit int) (events []gomatrixserverlib.ApplicationServiceEvent, maxID, txnID int, err error) {
	// Get current time for use in calculating event age
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)

	// Iterate through each row and store event contents
	// If txn_id changes dramatically, we've switched from collecting old events to
	// new ones. Send back those events first.
	lastTxnID := -2 // Invalid transaction ID
	for eventsProcessed := 0; eventRows.Next(); {
		var event gomatrixserverlib.ApplicationServiceEvent
		var eventContent sql.NullString
		var id int
		err = eventRows.Scan(
			&id,
			&event.EventID,
			&event.OriginServerTimestamp,
			&event.RoomID,
			&event.Type,
			&event.UserID,
			&eventContent,
			&txnID,
		)
		if err != nil {
			return nil, 0, 0, err
		}

		// If txnID has changed on this event from the previous event, then we've
		// reached the end of a transaction's events. Return only those events.
		if lastTxnID > -2 && lastTxnID != txnID {
			return events, maxID, lastTxnID, nil
		}
		lastTxnID = txnID

		// Limit events that aren't part of an old transaction
		if txnID == -1 {
			// Return if we've hit the limit
			if eventsProcessed++; eventsProcessed > limit {
				return events, maxID, lastTxnID, nil
			}
		}

		if eventContent.Valid {
			event.Content = gomatrixserverlib.RawJSON(eventContent.String)
		}
		if id > maxID {
			maxID = id
		}

		// Portion of the event that is unsigned due to rapid change
		event.Unsigned = gomatrixserverlib.ApplicationServiceUnsigned{
			// Get age of the event from original timestamp and current time
			// TODO: Consider removing age as not many app services use it
			Age: nowMilli - event.OriginServerTimestamp,
		}

		// TODO: Synapse does this. It's unnecessary to send Sender and UserID as the
		// same value. Do app services really require this? :)
		event.Sender = event.UserID

		events = append(events, event)
	}

	return
}

// countEventsByApplicationServiceID inserts an event mapped to its corresponding application service
// IDs into the db.
func (s *eventsStatements) countEventsByApplicationServiceID(
	ctx context.Context,
	appServiceID string,
) (int, error) {
	var count int
	err := s.countEventsByApplicationServiceIDStmt.QueryRowContext(ctx, appServiceID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}

	return count, nil
}

// insertEvent inserts an event mapped to its corresponding application service
// IDs into the db.
func (s *eventsStatements) insertEvent(
	ctx context.Context,
	appServiceID string,
	event *gomatrixserverlib.Event,
) (err error) {
	_, err = s.insertEventStmt.ExecContext(
		ctx,
		appServiceID,
		event.EventID(),
		event.OriginServerTS(),
		event.RoomID(),
		event.Type(),
		event.Sender(),
		event.Content(),
		-1, // No transaction ID yet
	)
	return
}

// updateTxnIDForEvents sets the transactionID for a collection of events. Done
// before sending them to an AppService. Referenced before sending to make sure
// we aren't constructing multiple transactions with the same events.
func (s *eventsStatements) updateTxnIDForEvents(
	ctx context.Context,
	appserviceID string,
	maxID, txnID int,
) (err error) {
	_, err = s.updateTxnIDForEventsStmt.ExecContext(ctx, txnID, appserviceID, maxID)
	return
}

// deleteEventsBeforeAndIncludingID removes events matching given IDs from the database.
func (s *eventsStatements) deleteEventsBeforeAndIncludingID(
	ctx context.Context,
	appserviceID string,
	eventTableID int,
) (err error) {
	_, err = s.deleteEventsBeforeAndIncludingIDStmt.ExecContext(ctx, appserviceID, eventTableID)
	return
}
