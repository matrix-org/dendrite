// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

const appserviceEventsSchema = `
-- Stores events to be sent to application services
CREATE TABLE IF NOT EXISTS appservice_events (
	-- An auto-incrementing id unique to each event in the table
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	-- The ID of the application service the event will be sent to
	as_id TEXT NOT NULL,
	-- JSON representation of the event
	headered_event_json TEXT NOT NULL,
	-- The ID of the transaction that this event is a part of
	txn_id INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS appservice_events_as_id ON appservice_events(as_id);
`

const selectEventsByApplicationServiceIDSQL = "" +
	"SELECT id, headered_event_json, txn_id " +
	"FROM appservice_events WHERE as_id = $1 ORDER BY txn_id DESC, id ASC"

const countEventsByApplicationServiceIDSQL = "" +
	"SELECT COUNT(id) FROM appservice_events WHERE as_id = $1"

const insertEventSQL = "" +
	"INSERT INTO appservice_events(as_id, headered_event_json, txn_id) " +
	"VALUES ($1, $2, $3)"

const updateTxnIDForEventsSQL = "" +
	"UPDATE appservice_events SET txn_id = $1 WHERE as_id = $2 AND id <= $3"

const deleteEventsBeforeAndIncludingIDSQL = "" +
	"DELETE FROM appservice_events WHERE as_id = $1 AND id <= $2"

const (
	// A transaction ID number that no transaction should ever have. Used for
	// checking again the default value.
	invalidTxnID = -2
)

type eventsStatements struct {
	db                                     *sql.DB
	writer                                 *sqlutil.TransactionWriter
	selectEventsByApplicationServiceIDStmt *sql.Stmt
	countEventsByApplicationServiceIDStmt  *sql.Stmt
	insertEventStmt                        *sql.Stmt
	updateTxnIDForEventsStmt               *sql.Stmt
	deleteEventsBeforeAndIncludingIDStmt   *sql.Stmt
}

func (s *eventsStatements) prepare(db *sql.DB) (err error) {
	s.db = db
	s.writer = sqlutil.NewTransactionWriter()
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
	events []gomatrixserverlib.HeaderedEvent,
	eventsRemaining bool,
	err error,
) {
	// Retrieve events from the database. Unsuccessfully sent events first
	eventRows, err := s.selectEventsByApplicationServiceIDStmt.QueryContext(ctx, applicationServiceID)
	if err != nil {
		return
	}
	defer func() {
		err = eventRows.Close()
		if err != nil {
			log.WithFields(log.Fields{
				"appservice": applicationServiceID,
			}).WithError(err).Fatalf("appservice unable to select new events to send")
		}
	}()
	events, maxID, txnID, eventsRemaining, err = retrieveEvents(eventRows, limit)
	if err != nil {
		return
	}

	return
}

func retrieveEvents(eventRows *sql.Rows, limit int) (events []gomatrixserverlib.HeaderedEvent, maxID, txnID int, eventsRemaining bool, err error) {
	// Get current time for use in calculating event age
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)

	// Iterate through each row and store event contents
	// If txn_id changes dramatically, we've switched from collecting old events to
	// new ones. Send back those events first.
	lastTxnID := invalidTxnID
	for eventsProcessed := 0; eventRows.Next(); {
		var event gomatrixserverlib.HeaderedEvent
		var eventJSON []byte
		var id int
		err = eventRows.Scan(
			&id,
			&eventJSON,
			&txnID,
		)
		if err != nil {
			return nil, 0, 0, false, err
		}

		// Unmarshal eventJSON
		if err = json.Unmarshal(eventJSON, &event); err != nil {
			return nil, 0, 0, false, err
		}

		// If txnID has changed on this event from the previous event, then we've
		// reached the end of a transaction's events. Return only those events.
		if lastTxnID > invalidTxnID && lastTxnID != txnID {
			return events, maxID, lastTxnID, true, nil
		}
		lastTxnID = txnID

		// Limit events that aren't part of an old transaction
		if txnID == -1 {
			// Return if we've hit the limit
			if eventsProcessed++; eventsProcessed > limit {
				return events, maxID, lastTxnID, true, nil
			}
		}

		if id > maxID {
			maxID = id
		}

		// Portion of the event that is unsigned due to rapid change
		// TODO: Consider removing age as not many app services use it
		if err = event.SetUnsignedField("age", nowMilli-int64(event.OriginServerTS())); err != nil {
			return nil, 0, 0, false, err
		}

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
	event *gomatrixserverlib.HeaderedEvent,
) (err error) {
	// Convert event to JSON before inserting
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		_, err := s.insertEventStmt.ExecContext(
			ctx,
			appServiceID,
			eventJSON,
			-1, // No transaction ID yet
		)
		return err
	})
}

// updateTxnIDForEvents sets the transactionID for a collection of events. Done
// before sending them to an AppService. Referenced before sending to make sure
// we aren't constructing multiple transactions with the same events.
func (s *eventsStatements) updateTxnIDForEvents(
	ctx context.Context,
	appserviceID string,
	maxID, txnID int,
) (err error) {
	return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		_, err := s.updateTxnIDForEventsStmt.ExecContext(ctx, txnID, appserviceID, maxID)
		return err
	})
}

// deleteEventsBeforeAndIncludingID removes events matching given IDs from the database.
func (s *eventsStatements) deleteEventsBeforeAndIncludingID(
	ctx context.Context,
	appserviceID string,
	eventTableID int,
) (err error) {
	return s.writer.Do(s.db, nil, func(txn *sql.Tx) error {
		_, err := s.deleteEventsBeforeAndIncludingIDStmt.ExecContext(ctx, appserviceID, eventTableID)
		return err
	})
}
