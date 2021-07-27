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

package cosmosdb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// const appserviceEventsSchema = `
// -- Stores events to be sent to application services
// CREATE TABLE IF NOT EXISTS appservice_events (
// 	-- An auto-incrementing id unique to each event in the table
// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
// 	-- The ID of the application service the event will be sent to
// 	as_id TEXT NOT NULL,
// 	-- JSON representation of the event
// 	headered_event_json TEXT NOT NULL,
// 	-- The ID of the transaction that this event is a part of
// 	txn_id INTEGER NOT NULL
// );

// CREATE INDEX IF NOT EXISTS appservice_events_as_id ON appservice_events(as_id);
// `

type EventCosmos struct {
	ID                int64  `json:"id"`
	AppServiceID      string `json:"as_id"`
	HeaderedEventJSON []byte `json:"headered_event_json"`
	TXNID             int64  `json:"txn_id"`
}

type EventNumberCosmosData struct {
	Number int `json:"number"`
}

type EventCosmosData struct {
	Id        string      `json:"id"`
	Pk        string      `json:"_pk"`
	Tn        string      `json:"_sid"`
	Cn        string      `json:"_cn"`
	ETag      string      `json:"_etag"`
	Timestamp int64       `json:"_ts"`
	Event     EventCosmos `json:"mx_appservice_event"`
}

// "SELECT id, headered_event_json, txn_id " +
// "FROM appservice_events WHERE as_id = $1 ORDER BY txn_id DESC, id ASC"
const selectEventsByApplicationServiceIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_appservice_event.as_id = @x2 " +
	"order by c.mx_appservice_event.txn_id desc " +
	"c.mx_appservice_event.id asc"

// "SELECT COUNT(id) FROM appservice_events WHERE as_id = $1"
const countEventsByApplicationServiceIDSQL = "" +
	"select count(c._ts) as number from c where c._cn = @x1 " +
	"and c.mx_appservice_event.as_id = @x2 "

// const insertEventSQL = "" +
// 	"INSERT INTO appservice_events(as_id, headered_event_json, txn_id) " +
// 	"VALUES ($1, $2, $3)"

// 	"UPDATE appservice_events SET txn_id = $1 WHERE as_id = $2 AND id <= $3"
const updateTxnIDForEventsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_appservice_event.as_id = @x2 " +
	"and c.mx_appservice_event.id <= @x3 "

// "DELETE FROM appservice_events WHERE as_id = $1 AND id <= $2"
const deleteEventsBeforeAndIncludingIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_appservice_event.as_id = @x2 " +
	"and c.mx_appservice_event.id <= @x3 "

const (
	// A transaction ID number that no transaction should ever have. Used for
	// checking again the default value.
	invalidTxnID = -2
)

type eventsStatements struct {
	db                                     *Database
	writer                                 sqlutil.Writer
	selectEventsByApplicationServiceIDStmt string
	countEventsByApplicationServiceIDStmt  string
	// insertEventStmt                        *sql.Stmt
	updateTxnIDForEventsStmt             string
	deleteEventsBeforeAndIncludingIDStmt string
	tableName                            string
}

func (s *eventsStatements) prepare(db *Database, writer sqlutil.Writer) (err error) {
	s.db = db
	s.writer = writer

	s.selectEventsByApplicationServiceIDStmt = selectEventsByApplicationServiceIDSQL
	s.countEventsByApplicationServiceIDStmt = countEventsByApplicationServiceIDSQL
	s.updateTxnIDForEventsStmt = updateTxnIDForEventsSQL
	s.deleteEventsBeforeAndIncludingIDStmt = deleteEventsBeforeAndIncludingIDSQL
	s.tableName = "events"
	return
}

func queryEvent(s *eventsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]EventCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []EventCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func queryEventEventNumber(s *eventsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]EventNumberCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []EventNumberCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(qry, params)
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return nil, err
	}
	return response, nil
}

func setEvent(s *eventsStatements, ctx context.Context, event EventCosmosData) (*EventCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(event.Pk, event.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		event.Id,
		&event,
		optionsReplace)
	return &event, ex
}

func deleteEvent(s *eventsStatements, ctx context.Context, event EventCosmosData) error {
	var options = cosmosdbapi.GetDeleteDocumentOptions(event.Pk)
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		event.Id,
		options)

	if err != nil {
		return err
	}
	return err
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

	// "SELECT id, headered_event_json, txn_id " +
	// "FROM appservice_events WHERE as_id = $1 ORDER BY txn_id DESC, id ASC"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": applicationServiceID,
	}

	eventRows, err := queryEvent(s, ctx, s.selectEventsByApplicationServiceIDStmt, params)

	if err != nil {
		log.WithFields(log.Fields{
			"appservice": applicationServiceID,
		}).WithError(err).Fatalf("appservice unable to select new events to send")
	}

	events, maxID, txnID, eventsRemaining, err = retrieveEvents(eventRows, limit)
	if err != nil {
		return
	}

	return
}

// checkNamedErr calls fn and overwrite err if it was nil and fn returned non-nil
func checkNamedErr(fn func() error, err *error) {
	if e := fn(); e != nil && *err == nil {
		*err = e
	}
}

func retrieveEvents(eventRows []EventCosmosData, limit int) (events []gomatrixserverlib.HeaderedEvent, maxID, txnID int, eventsRemaining bool, err error) {
	// Get current time for use in calculating event age
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)

	// Iterate through each row and store event contents
	// If txn_id changes dramatically, we've switched from collecting old events to
	// new ones. Send back those events first.
	lastTxnID := invalidTxnID
	for eventsProcessed := 0; eventsProcessed < len(eventRows); {
		var event gomatrixserverlib.HeaderedEvent
		var eventJSON []byte
		var id int
		item := eventRows[eventsProcessed]
		id = int(item.Event.ID)
		eventJSON = item.Event.HeaderedEventJSON
		txnID = int(item.Event.TXNID)
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

	// "SELECT COUNT(id) FROM appservice_events WHERE as_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": appServiceID,
	}

	response, err := queryEventEventNumber(s, ctx, s.countEventsByApplicationServiceIDStmt, params)

	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	count = response[0].Number

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

	// "INSERT INTO appservice_events(as_id, headered_event_json, txn_id) " +
	// "VALUES ($1, $2, $3)"

	// 	id INTEGER PRIMARY KEY AUTOINCREMENT,
	idSeq, seqErr := GetNextEventID(s, ctx)
	if seqErr != nil {
		return seqErr
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := fmt.Sprintf("%d", idSeq)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// appServiceID,
	// eventJSON,
	// -1, // No transaction ID yet
	data := EventCosmos{
		AppServiceID:      appServiceID,
		HeaderedEventJSON: eventJSON,
		ID:                idSeq,
		TXNID:             -1,
	}

	dbData := &EventCosmosData{
		Id:        cosmosDocId,
		Tn:        s.db.cosmosConfig.TenantName,
		Cn:        dbCollectionName,
		Pk:        pk,
		Timestamp: time.Now().Unix(),
		Event:     data,
	}

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err

}

// updateTxnIDForEvents sets the transactionID for a collection of events. Done
// before sending them to an AppService. Referenced before sending to make sure
// we aren't constructing multiple transactions with the same events.
func (s *eventsStatements) updateTxnIDForEvents(
	ctx context.Context,
	appserviceID string,
	maxID, txnID int,
) (err error) {
	// "UPDATE appservice_events SET txn_id = $1 WHERE as_id = $2 AND id <= $3"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": appserviceID,
		"@x3": maxID,
	}

	response, err := queryEvent(s, ctx, s.updateTxnIDForEventsStmt, params)
	if err != nil {
		return err
	}

	for _, item := range response {
		item.Event.TXNID = int64(txnID)
		// _, err := s.updateTxnIDForEventsStmt.ExecContext(ctx, txnID, appserviceID, maxID)
		_, err = setEvent(s, ctx, item)
	}

	return err
}

// deleteEventsBeforeAndIncludingID removes events matching given IDs from the database.
func (s *eventsStatements) deleteEventsBeforeAndIncludingID(
	ctx context.Context,
	appserviceID string,
	eventTableID int,
) (err error) {
	// "DELETE FROM appservice_events WHERE as_id = $1 AND id <= $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": appserviceID,
		"@x3": eventTableID,
	}

	response, err := queryEvent(s, ctx, s.deleteEventsBeforeAndIncludingIDStmt, params)
	if err != nil {
		return err
	}

	for _, item := range response {
		// _, err := s.updateTxnIDForEventsStmt.ExecContext(ctx, txnID, appserviceID, maxID)
		err = deleteEvent(s, ctx, item)
	}
	return err
}
