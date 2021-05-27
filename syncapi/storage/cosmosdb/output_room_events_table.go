// Copyright 2017-2018 New Vector Ltd
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
	"sort"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"

	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// const outputRoomEventsSchema = `
// -- Stores output room events received from the roomserver.
// CREATE TABLE IF NOT EXISTS syncapi_output_room_events (
//   id INTEGER PRIMARY KEY AUTOINCREMENT,
//   event_id TEXT NOT NULL UNIQUE,
//   room_id TEXT NOT NULL,
//   headered_event_json TEXT NOT NULL,
//   type TEXT NOT NULL,
//   sender TEXT NOT NULL,
//   contains_url BOOL NOT NULL,
//   add_state_ids TEXT, -- JSON encoded string array
//   remove_state_ids TEXT, -- JSON encoded string array
//   session_id BIGINT,
//   transaction_id TEXT,
//   exclude_from_sync BOOL NOT NULL DEFAULT FALSE
// );
// `

type OutputRoomEventCosmos struct {
	ID                int64  `json:"id"`
	EventID           string `json:"event_id"`
	RoomID            string `json:"room_id"`
	HeaderedEventJSON []byte `json:"headered_event_json"`
	Type              string `json:"type"`
	Sender            string `json:"sender"`
	ContainsUrl       bool   `json:"contains_url"`
	AddStateIDs       string `json:"add_state_ids"`
	RemoveStateIDs    string `json:"remove_state_ids"`
	SessionID         int64  `json:"session_id"`
	TransactionID     string `json:"transaction_id"`
	ExcludeFromSync   bool   `json:"exclude_from_sync"`
}

type OutputRoomEventCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type OutputRoomEventCosmosData struct {
	Id              string                `json:"id"`
	Pk              string                `json:"_pk"`
	Cn              string                `json:"_cn"`
	ETag            string                `json:"_etag"`
	Timestamp       int64                 `json:"_ts"`
	OutputRoomEvent OutputRoomEventCosmos `json:"mx_syncapi_output_room_event"`
}

// const insertEventSQL = "" +
// 	"INSERT INTO syncapi_output_room_events (" +
// 	"id, room_id, event_id, headered_event_json, type, sender, contains_url, add_state_ids, remove_state_ids, session_id, transaction_id, exclude_from_sync" +
// 	") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) " +
// 	"ON CONFLICT (event_id) DO UPDATE SET exclude_from_sync = (excluded.exclude_from_sync AND $13)"

// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events WHERE event_id = $1"
const selectEventsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.event_id = @x2 "

// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
// " WHERE room_id = $1 AND id > $2 AND id <= $3"
// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters
const selectRecentEventsSQL = "" +
	"select top @x5 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.room_id = @x2 " +
	"and c.mx_syncapi_output_room_event.id > @x3 " +
	"and c.mx_syncapi_output_room_event.id <= @x4 "

// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
// " WHERE room_id = $1 AND id > $2 AND id <= $3 AND exclude_from_sync = FALSE"
// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters
const selectRecentEventsForSyncSQL = "" +
	"select top @x5 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.room_id = @x2 " +
	"and c.mx_syncapi_output_room_event.id > @x3 " +
	"and c.mx_syncapi_output_room_event.id <= @x4 " +
	"and c.mx_syncapi_output_room_event.exclude_from_sync = false "

// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
// " WHERE room_id = $1 AND id > $2 AND id <= $3"
// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters
const selectEarlyEventsSQL = "" +
	"select top @x5 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.room_id = @x2 " +
	"and c.mx_syncapi_output_room_event.id > @x3 " +
	"and c.mx_syncapi_output_room_event.id <= @x4 "

// "SELECT MAX(id) FROM syncapi_output_room_events"
const selectMaxEventIDSQL = "" +
	"select max(c.mx_syncapi_output_room_event.id) as number from c where c._cn = @x1 "

// "UPDATE syncapi_output_room_events SET headered_event_json=$1 WHERE event_id=$2"
const updateEventJSONSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.event_id = @x2 "

// "SELECT id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids" +
// " FROM syncapi_output_room_events" +
// " WHERE (id > $1 AND id <= $2)" +
// " AND ((add_state_ids IS NOT NULL AND add_state_ids != '') OR (remove_state_ids IS NOT NULL AND remove_state_ids != ''))"
// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters
const selectStateInRangeSQL = "" +
	"select top @x4 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.id > @x2 " +
	"and c.mx_syncapi_output_room_event.id <= @x3 " +
	"and (c.mx_syncapi_output_room_event.add_state_ids != null or c.mx_syncapi_output_room_event.remove_state_ids != null) "

	// "DELETE FROM syncapi_output_room_events WHERE room_id = $1"
const deleteEventsForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event.room_id = @x2 "

type outputRoomEventsStatements struct {
	db                 *SyncServerDatasource
	streamIDStatements *streamIDStatements
	// insertEventStmt         *sql.Stmt
	selectEventsStmt        string
	selectMaxEventIDStmt    string
	updateEventJSONStmt     string
	deleteEventsForRoomStmt string
	tableName               string
	jsonPropertyName        string
}

func queryOutputRoomEvent(s *outputRoomEventsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]OutputRoomEventCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []OutputRoomEventCosmosData

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

func queryOutputRoomEventNumber(s *outputRoomEventsStatements, ctx context.Context, qry string, params map[string]interface{}) ([]OutputRoomEventCosmosMaxNumber, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []OutputRoomEventCosmosMaxNumber

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
		return nil, cosmosdbutil.ErrNoRows
	}
	return response, nil
}

func setOutputRoomEvent(s *outputRoomEventsStatements, ctx context.Context, outputRoomEvent OutputRoomEventCosmosData) (*OutputRoomEventCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(outputRoomEvent.Pk, outputRoomEvent.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		outputRoomEvent.Id,
		&outputRoomEvent,
		optionsReplace)
	return &outputRoomEvent, ex
}

func deleteOutputRoomEvent(s *outputRoomEventsStatements, ctx context.Context, dbData OutputRoomEventCosmosData) error {
	var options = cosmosdbapi.GetDeleteDocumentOptions(dbData.Pk)
	var _, err = cosmosdbapi.GetClient(s.db.connection).DeleteDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Id,
		options)

	if err != nil {
		return err
	}
	return err
}

func NewCosmosDBEventsTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.Events, error) {
	s := &outputRoomEventsStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	s.selectEventsStmt = selectEventsSQL
	s.selectMaxEventIDStmt = selectMaxEventIDSQL
	s.updateEventJSONStmt = updateEventJSONSQL
	s.deleteEventsForRoomStmt = deleteEventsForRoomSQL
	s.tableName = "output_room_events"
	s.jsonPropertyName = "mx_syncapi_output_room_event"
	return s, nil
}

func (s *outputRoomEventsStatements) UpdateEventJSON(ctx context.Context, event *gomatrixserverlib.HeaderedEvent) error {
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// "UPDATE syncapi_output_room_events SET headered_event_json=$1 WHERE event_id=$2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": event.EventID(),
	}

	// _, err = s.updateEventJSONStmt.ExecContext(ctx, headeredJSON, event.EventID())
	rows, err := queryOutputRoomEvent(s, ctx, s.deleteEventsForRoomStmt, params)
	if err != nil {
		return err
	}

	for _, item := range rows {
		item.OutputRoomEvent.HeaderedEventJSON = headeredJSON
		_, err = setOutputRoomEvent(s, ctx, item)
	}

	return err
	return err
}

// selectStateInRange returns the state events between the two given PDU stream positions, exclusive of oldPos, inclusive of newPos.
// Results are bucketed based on the room ID. If the same state is overwritten multiple times between the
// two positions, only the most recent state is returned.
func (s *outputRoomEventsStatements) SelectStateInRange(
	ctx context.Context, txn *sql.Tx, r types.Range,
	stateFilter *gomatrixserverlib.StateFilter,
) (map[string]map[string]bool, map[string]types.StreamEvent, error) {
	// "SELECT id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids" +
	// " FROM syncapi_output_room_events" +
	// " WHERE (id > $1 AND id <= $2)" +
	// " AND ((add_state_ids IS NOT NULL AND add_state_ids != '') OR (remove_state_ids IS NOT NULL AND remove_state_ids != ''))"
	// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": r.Low(),
		"@x3": r.High(),
		"@x4": stateFilter.Limit,
	}
	query, params := prepareWithFilters(
		s.jsonPropertyName, selectStateInRangeSQL, params,
		stateFilter.Senders, stateFilter.NotSenders,
		stateFilter.Types, stateFilter.NotTypes,
		nil, stateFilter.Limit, FilterOrderAsc,
	)

	// rows, err := stmt.QueryContext(ctx, params...)
	rows, err := queryOutputRoomEvent(s, ctx, query, params)
	if err != nil {
		return nil, nil, err
	}
	// Fetch all the state change events for all rooms between the two positions then loop each event and:
	//  - Keep a cache of the event by ID (99% of state change events are for the event itself)
	//  - For each room ID, build up an array of event IDs which represents cumulative adds/removes
	// For each room, map cumulative event IDs to events and return. This may need to a batch SELECT based on event ID
	// if they aren't in the event ID cache. We don't handle state deletion yet.
	eventIDToEvent := make(map[string]types.StreamEvent)

	// RoomID => A set (map[string]bool) of state event IDs which are between the two positions
	stateNeeded := make(map[string]map[string]bool)

	for _, item := range rows {
		var (
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			addIDsJSON      string
			delIDsJSON      string
		)
		// SELECT id, headered_event_json, exclude_from_sync, add_state_ids, remove_state_ids
		// if err := rows.Scan(&streamPos, &eventBytes, &excludeFromSync, &addIDsJSON, &delIDsJSON); err != nil {
		// 	return nil, nil, err
		// }
		streamPos = types.StreamPosition(item.OutputRoomEvent.ID)
		eventBytes = item.OutputRoomEvent.HeaderedEventJSON
		excludeFromSync = item.OutputRoomEvent.ExcludeFromSync
		addIDsJSON = item.OutputRoomEvent.AddStateIDs
		delIDsJSON = item.OutputRoomEvent.RemoveStateIDs
		addIDs, delIDs, err := unmarshalStateIDs(addIDsJSON, delIDsJSON)
		if err != nil {
			return nil, nil, err
		}

		// Sanity check for deleted state and whine if we see it. We don't need to do anything
		// since it'll just mark the event as not being needed.
		if len(addIDs) < len(delIDs) {
			log.WithFields(log.Fields{
				"since":   r.From,
				"current": r.To,
				"adds":    addIDsJSON,
				"dels":    delIDsJSON,
			}).Warn("StateBetween: ignoring deleted state")
		}

		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(eventBytes, &ev); err != nil {
			return nil, nil, err
		}
		needSet := stateNeeded[ev.RoomID()]
		if needSet == nil { // make set if required
			needSet = make(map[string]bool)
		}
		for _, id := range delIDs {
			needSet[id] = false
		}
		for _, id := range addIDs {
			needSet[id] = true
		}
		stateNeeded[ev.RoomID()] = needSet

		eventIDToEvent[ev.EventID()] = types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			ExcludeFromSync: excludeFromSync,
		}
	}

	return stateNeeded, eventIDToEvent, nil
}

// MaxID returns the ID of the last inserted event in this table. 'txn' is optional. If it is not supplied,
// then this function should only ever be used at startup, as it will race with inserting events if it is
// done afterwards. If there are no inserted events, 0 is returned.
func (s *outputRoomEventsStatements) SelectMaxEventID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}
	// stmt := sqlutil.TxStmt(txn, s.selectMaxEventIDStmt)

	rows, err := queryOutputRoomEventNumber(s, ctx, s.selectMaxEventIDStmt, params)
	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)

	if rows != nil {
		nullableID.Int64 = rows[0].Max
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}

// InsertEvent into the output_room_events table. addState and removeState are an optional list of state event IDs. Returns the position
// of the inserted event.
func (s *outputRoomEventsStatements) InsertEvent(
	ctx context.Context, txn *sql.Tx,
	event *gomatrixserverlib.HeaderedEvent, addState, removeState []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (types.StreamPosition, error) {

	var txnID *string
	var sessionID *int64
	if transactionID != nil {
		sessionID = &transactionID.SessionID
		txnID = &transactionID.TransactionID
	}

	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	var headeredJSON []byte
	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}

	var addStateJSON, removeStateJSON []byte
	if len(addState) > 0 {
		addStateJSON, err = json.Marshal(addState)
	}
	if err != nil {
		return 0, fmt.Errorf("json.Marshal(addState): %w", err)
	}
	if len(removeState) > 0 {
		removeStateJSON, err = json.Marshal(removeState)
	}
	if err != nil {
		return 0, fmt.Errorf("json.Marshal(removeState): %w", err)
	}

	//   id INTEGER PRIMARY KEY AUTOINCREMENT,
	streamPos, err := s.streamIDStatements.nextPDUID(ctx, txn)
	if err != nil {
		return 0, err
	}
	// "INSERT INTO syncapi_output_room_events (" +
	// "id, room_id, event_id, headered_event_json, type, sender, contains_url, add_state_ids, remove_state_ids, session_id, transaction_id, exclude_from_sync" +
	// ") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) " +
	// "ON CONFLICT (event_id) DO UPDATE SET exclude_from_sync = (excluded.exclude_from_sync AND $13)"

	// insertStmt := sqlutil.TxStmt(txn, s.insertEventStmt)
	// _, err = insertStmt.ExecContext(
	// 	ctx,
	// 	streamPos,
	// 	event.RoomID(),
	// 	event.EventID(),
	// 	headeredJSON,
	// 	event.Type(),
	// 	event.Sender(),
	// 	containsURL,
	// 	string(addStateJSON),
	// 	string(removeStateJSON),
	// 	sessionID,
	// 	txnID,
	// 	excludeFromSync,
	// 	excludeFromSync,
	// )

	data := OutputRoomEventCosmos{
		ID:                int64(streamPos),
		RoomID:            event.RoomID(),
		EventID:           event.EventID(),
		HeaderedEventJSON: headeredJSON,
		Type:              event.Type(),
		Sender:            event.Sender(),
		ContainsUrl:       containsURL,
		AddStateIDs:       string(addStateJSON),
		RemoveStateIDs:    string(removeStateJSON),
		ExcludeFromSync:   excludeFromSync,
	}

	if transactionID != nil {
		data.SessionID = *sessionID
		data.TransactionID = *txnID
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	// id INTEGER PRIMARY KEY,
	docId := fmt.Sprintf("%d", streamPos)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)

	var dbData = OutputRoomEventCosmosData{
		Id:              cosmosDocId,
		Cn:              dbCollectionName,
		Pk:              pk,
		Timestamp:       time.Now().Unix(),
		OutputRoomEvent: data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	return streamPos, err
}

func (s *outputRoomEventsStatements) SelectRecentEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter,
	chronologicalOrder bool, onlySyncEvents bool,
) ([]types.StreamEvent, bool, error) {
	var query string
	if onlySyncEvents {
		// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
		// " WHERE room_id = $1 AND id > $2 AND id <= $3"
		// // WHEN, ORDER BY and LIMIT are appended by prepareWithFilters
		query = selectRecentEventsForSyncSQL
	} else {
		// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
		// " WHERE room_id = $1 AND id > $2 AND id <= $3" +
		query = selectRecentEventsSQL
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": r.Low(),
		"@x4": r.High(),
		"@x5": eventFilter.Limit + 1,
	}

	query, params = prepareWithFilters(
		s.jsonPropertyName, query, params,
		eventFilter.Senders, eventFilter.NotSenders,
		eventFilter.Types, eventFilter.NotTypes,
		nil, eventFilter.Limit+1, FilterOrderDesc,
	)

	// rows, err := stmt.QueryContext(ctx, params...)
	rows, err := queryOutputRoomEvent(s, ctx, query, params)

	if err != nil {
		return nil, false, err
	}
	events, err := rowsToStreamEvents(&rows)
	if err != nil {
		return nil, false, err
	}
	if chronologicalOrder {
		// The events need to be returned from oldest to latest, which isn't
		// necessary the way the SQL query returns them, so a sort is necessary to
		// ensure the events are in the right order in the slice.
		sort.SliceStable(events, func(i int, j int) bool {
			return events[i].StreamPosition < events[j].StreamPosition
		})
	}
	// we queried for 1 more than the limit, so if we returned one more mark limited=true
	limited := false
	if len(events) > eventFilter.Limit {
		limited = true
		// re-slice the extra (oldest) event out: in chronological order this is the first entry, else the last.
		if chronologicalOrder {
			events = events[1:]
		} else {
			events = events[:len(events)-1]
		}
	}
	return events, limited, nil
}

func (s *outputRoomEventsStatements) SelectEarlyEvents(
	ctx context.Context, txn *sql.Tx,
	roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter,
) ([]types.StreamEvent, error) {
	// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events" +
	// " WHERE room_id = $1 AND id > $2 AND id <= $3"
	// // WHEN, ORDER BY (and not LIMIT) are appended by prepareWithFilters

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": r.Low(),
		"@x4": r.High(),
		"@x5": eventFilter.Limit,
	}
	stmt, params := prepareWithFilters(
		s.jsonPropertyName, selectEarlyEventsSQL, params,
		eventFilter.Senders, eventFilter.NotSenders,
		eventFilter.Types, eventFilter.NotTypes,
		nil, eventFilter.Limit, FilterOrderAsc,
	)

	// rows, err := stmt.QueryContext(ctx, params...)
	rows, err := queryOutputRoomEvent(s, ctx, stmt, params)
	if err != nil {
		return nil, err
	}
	events, err := rowsToStreamEvents(&rows)
	if err != nil {
		return nil, err
	}
	// The events need to be returned from oldest to latest, which isn't
	// necessarily the way the SQL query returns them, so a sort is necessary to
	// ensure the events are in the right order in the slice.
	sort.SliceStable(events, func(i int, j int) bool {
		return events[i].StreamPosition < events[j].StreamPosition
	})
	return events, nil
}

// selectEvents returns the events for the given event IDs. If an event is
// missing from the database, it will be omitted.
func (s *outputRoomEventsStatements) SelectEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	// "SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id FROM syncapi_output_room_events WHERE event_id = $1"

	var returnEvents []types.StreamEvent

	// stmt := sqlutil.TxStmt(txn, s.selectEventsStmt)

	for _, eventID := range eventIDs {
		var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
		params := map[string]interface{}{
			"@x1": dbCollectionName,
			"@x2": eventID,
		}

		// rows, err := stmt.QueryContext(ctx, eventID)
		rows, err := queryOutputRoomEvent(s, ctx, s.selectEventsStmt, params)
		if err != nil {
			return nil, err
		}
		if streamEvents, err := rowsToStreamEvents(&rows); err == nil {
			returnEvents = append(returnEvents, streamEvents...)
		}
	}
	return returnEvents, nil
}

func (s *outputRoomEventsStatements) DeleteEventsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	// "DELETE FROM syncapi_output_room_events WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteEventsForRoomStmt).ExecContext(ctx, roomID)
	rows, err := queryOutputRoomEvent(s, ctx, s.deleteEventsForRoomStmt, params)
	if err != nil {
		return err
	}

	for _, item := range rows {
		err = deleteOutputRoomEvent(s, ctx, item)
	}

	return err
}

func rowsToStreamEvents(rows *[]OutputRoomEventCosmosData) ([]types.StreamEvent, error) {
	var result []types.StreamEvent
	for _, item := range *rows {
		var (
			eventID         string
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			sessionID       *int64
			txnID           *string
			transactionID   *api.TransactionID
		)
		// SELECT event_id, id, headered_event_json, session_id, exclude_from_sync, transaction_id
		// if err := rows.Scan(&eventID, &streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID); err != nil {
		// 	return nil, err
		// }
		eventID = item.OutputRoomEvent.EventID
		streamPos = types.StreamPosition(item.OutputRoomEvent.ID)
		eventBytes = item.OutputRoomEvent.HeaderedEventJSON
		sessionID = &item.OutputRoomEvent.SessionID
		excludeFromSync = item.OutputRoomEvent.ExcludeFromSync
		txnID = &item.OutputRoomEvent.TransactionID

		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}

		if sessionID != nil && txnID != nil {
			transactionID = &api.TransactionID{
				SessionID:     *sessionID,
				TransactionID: *txnID,
			}
		}

		result = append(result, types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			TransactionID:   transactionID,
			ExcludeFromSync: excludeFromSync,
		})
	}
	return result, nil
}

func unmarshalStateIDs(addIDsJSON, delIDsJSON string) (addIDs []string, delIDs []string, err error) {
	if len(addIDsJSON) > 0 {
		if err = json.Unmarshal([]byte(addIDsJSON), &addIDs); err != nil {
			return
		}
	}
	if len(delIDsJSON) > 0 {
		if err = json.Unmarshal([]byte(delIDsJSON), &delIDs); err != nil {
			return
		}
	}
	return
}
