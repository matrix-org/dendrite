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
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const eventsSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_events (
//     event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
//     room_nid INTEGER NOT NULL,
//     event_type_nid INTEGER NOT NULL,
//     event_state_key_nid INTEGER NOT NULL,
//     sent_to_output BOOLEAN NOT NULL DEFAULT FALSE,
//     state_snapshot_nid INTEGER NOT NULL DEFAULT 0,
//     depth INTEGER NOT NULL,
//     event_id TEXT NOT NULL UNIQUE,
//     reference_sha256 BLOB NOT NULL,
// 	auth_event_nids TEXT NOT NULL DEFAULT '[]',
// 	is_rejected BOOLEAN NOT NULL DEFAULT FALSE
//   );
// `

type EventCosmos struct {
	EventNID         int64   `json:"event_nid"`
	RoomNID          int64   `json:"room_nid"`
	EventTypeNID     int64   `json:"event_type_nid"`
	EventStateKeyNID int64   `json:"event_state_key_nid"`
	SentToOutput     bool    `json:"sent_to_output"`
	StateSnapshotNID int64   `json:"state_snapshot_nid"`
	Depth            int64   `json:"depth"`
	EventId          string  `json:"event_id"`
	ReferenceSha256  []byte  `json:"reference_sha256"`
	AuthEventNIDs    []int64 `json:"auth_event_nids"`
	IsRejected       bool    `json:"is_rejected"`
}

type EventCosmosMaxDepth struct {
	Max int64 `json:"maxdepth"`
}

type EventCosmosData struct {
	Id        string      `json:"id"`
	Pk        string      `json:"_pk"`
	Tn        string      `json:"_sid"`
	Cn        string      `json:"_cn"`
	ETag      string      `json:"_etag"`
	Timestamp int64       `json:"_ts"`
	Event     EventCosmos `json:"mx_roomserver_event"`
}

// const insertEventSQL = `
// 	INSERT INTO roomserver_events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids, depth, is_rejected)
// 	  VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
// 	  ON CONFLICT DO NOTHING;
// `

// 	"SELECT event_nid, state_snapshot_nid FROM roomserver_events WHERE event_id = $1"
// const selectEventSQL = "" +
// 	"select * from c where c._cn = @x1 and c.mx_roomserver_event.event_id = @x2"

// Bulk lookup of events by string ID.
// Sort by the numeric IDs for event type and state key.
// This means we can use binary search to lookup entries by type and state key.
// 	"SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
// 	" WHERE event_id IN ($1)" +
// 	" ORDER BY event_type_nid, event_state_key_nid ASC"
const bulkSelectStateEventByIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_id) " +
	"order by c.mx_roomserver_event.event_type_nid " +
	// Cant do multi field order by - The order by query does not have a corresponding composite index that it can be served from
	// ", c.mx_roomserver_event.event_state_key_nid " +
	"asc"

// 	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, is_rejected FROM roomserver_events" +
// 	" WHERE event_id IN ($1)"
const bulkSelectStateAtEventByIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_id)"

// 	"UPDATE roomserver_events SET state_snapshot_nid = $1 WHERE event_nid = $2"
const updateEventStateSQL = "" +
	"select * from c where c._cn = @x1 and c.mx_roomserver_event.event_nid = @x2"

// "SELECT sent_to_output FROM roomserver_events WHERE event_nid = $1"
const selectEventSentToOutputSQL = "" +
	"select * from c where c._cn = @x1 and c.mx_roomserver_event.event_nid = @x2"

// 	"UPDATE roomserver_events SET sent_to_output = TRUE WHERE event_nid = $1"
const updateEventSentToOutputSQL = "" +
	"select * from c where c._cn = @x1 and c.mx_roomserver_event.event_nid = @x2"

// "SELECT event_id FROM roomserver_events WHERE event_nid = $1"
const selectEventIDSQL = "" +
	"select * from c where c._cn = @x1 and c.mx_roomserver_event.event_nid = @x2"

// 	"SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, event_id, reference_sha256" +
// 	" FROM roomserver_events WHERE event_nid IN ($1)"
const bulkSelectStateAtEventAndReferenceSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid)"

// 	"SELECT event_id, reference_sha256 FROM roomserver_events WHERE event_nid IN ($1)"
const bulkSelectEventReferenceSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid)"

// 	"SELECT event_nid, event_id FROM roomserver_events WHERE event_nid IN ($1)"
const bulkSelectEventIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid)"

// 	"SELECT event_id, event_nid FROM roomserver_events WHERE event_id IN ($1)"
const bulkSelectEventNIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_id)"

// 	"SELECT COALESCE(MAX(depth) + 1, 0) FROM roomserver_events WHERE event_nid IN ($1)"
const selectMaxEventDepthSQL = "" +
	"select sub.maxinner != null ? sub.maxinner + 1 : 0 as maxdepth from " +
	"(select MAX(c.mx_roomserver_event.depth) maxinner from c where c._sid = @x1 and c._cn = @x2 " +
	" and ARRAY_CONTAINS(@x3, c.mx_roomserver_event.event_nid)) sub"

// 	"SELECT event_nid, room_nid FROM roomserver_events WHERE event_nid IN ($1)"
const selectRoomNIDsForEventNIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid)"

type eventStatements struct {
	db *Database
	// insertEventStmt                        *sql.Stmt
	// selectEventStmt                string
	bulkSelectStateEventByIDStmt           string
	bulkSelectStateAtEventByIDStmt         string
	updateEventStateStmt                   string
	selectEventSentToOutputStmt            string
	updateEventSentToOutputStmt            string
	selectEventIDStmt                      string
	bulkSelectStateAtEventAndReferenceStmt string
	bulkSelectEventReferenceStmt           string
	bulkSelectEventIDStmt                  string
	bulkSelectEventNIDStmt                 string
	// selectRoomNIDsForEventNIDsStmt         string
	tableName string
}

func NewCosmosDBEventsTable(db *Database) (tables.Events, error) {
	s := &eventStatements{
		db: db,
	}
	// _, err := db.Exec(eventsSchema)
	// if err != nil {
	// 	return nil, err
	// }
	s.tableName = "events"
	// return s, shared.StatementList{
	// {&s.insertEventStmt, insertEventSQL},
	// s.selectEventStmt = selectEventSQL
	s.bulkSelectStateEventByIDStmt = bulkSelectStateEventByIDSQL
	s.bulkSelectStateAtEventByIDStmt = bulkSelectStateAtEventByIDSQL
	s.updateEventStateStmt = updateEventStateSQL
	s.updateEventSentToOutputStmt = updateEventSentToOutputSQL
	s.selectEventSentToOutputStmt = selectEventSentToOutputSQL
	s.selectEventIDStmt = selectEventIDSQL
	s.bulkSelectStateAtEventAndReferenceStmt = bulkSelectStateAtEventAndReferenceSQL
	s.bulkSelectEventReferenceStmt = bulkSelectEventReferenceSQL
	s.bulkSelectEventIDStmt = bulkSelectEventIDSQL
	s.bulkSelectEventNIDStmt = bulkSelectEventNIDSQL
	// }.Prepare(db)
	return s, nil
}

func mapFromEventNIDArray(eventNIDs []types.EventNID) []int64 {
	result := []int64{}
	for i := 0; i < len(eventNIDs); i++ {
		result = append(result, int64(eventNIDs[i]))
	}
	return result
}

func queryEvent(s *eventStatements, ctx context.Context, qry string, params map[string]interface{}) ([]EventCosmosData, error) {
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

func getEvent(s *eventStatements, ctx context.Context, pk string, docId string) (*EventCosmosData, error) {
	response := EventCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, cosmosdbutil.ErrNoRows
	}

	return &response, err
}

func setEvent(s *eventStatements, ctx context.Context, event EventCosmosData) (*EventCosmosData, error) {
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

func isEventAuthEventNIDsSame(
	ids []int64,
	authEventNIDs []types.EventNID,
) bool {
	if len(ids) != len(authEventNIDs) {
		return false
	}
	for i := 0; i < len(ids); i++ {
		if ids[i] != int64(authEventNIDs[i]) {
			return false
		}
	}
	return true
}

func isReferenceSha256Same(
	ids []byte,
	referenceSHA256 []byte,
) bool {
	if len(ids) != len(referenceSHA256) {
		return false
	}
	for i := 0; i < len(ids); i++ {
		if ids[i] != referenceSHA256[i] {
			return false
		}
	}
	return true
}

func isEventSame(
	event EventCosmos,
	roomNID types.RoomNID,
	eventTypeNID types.EventTypeNID,
	eventStateKeyNID types.EventStateKeyNID,
	eventID string,
	referenceSHA256 []byte,
	authEventNIDs []types.EventNID,
	depth int64,
	isRejected bool,
) bool {
	if event.RoomNID != int64(roomNID) {
		return false
	}
	if event.EventTypeNID != int64(eventTypeNID) {
		return false
	}
	if event.EventStateKeyNID != int64(eventStateKeyNID) {
		return false
	}
	if event.EventId != eventID {
		return false
	}
	if isReferenceSha256Same(event.ReferenceSha256, referenceSHA256) {
		return false
	}
	if !isEventAuthEventNIDsSame(event.AuthEventNIDs, authEventNIDs) {
		return false
	}
	if event.Depth != depth {
		return false
	}
	if event.IsRejected != isRejected {
		return false
	}
	return true
}

func (s *eventStatements) InsertEvent(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventTypeNID types.EventTypeNID,
	eventStateKeyNID types.EventStateKeyNID,
	eventID string,
	referenceSHA256 []byte,
	authEventNIDs []types.EventNID,
	depth int64,
	isRejected bool,
) (types.EventNID, types.StateSnapshotNID, error) {

	// INSERT INTO roomserver_events (room_nid, event_type_nid, event_state_key_nid, event_id, reference_sha256, auth_event_nids, depth, is_rejected)
	// VALUES ($1, $2, $3, $4, $5, $6, $7, $8)

	//     event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	//     event_id TEXT NOT NULL UNIQUE,
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := eventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	dbData, errGet := getEvent(s, ctx, pk, cosmosDocId)

	// ON CONFLICT DO NOTHING;
	//     event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	var eventNID int64
	if errGet == cosmosdbutil.ErrNoRows {
		eventNIDSeq, seqErr := GetNextEventNID(s, ctx)
		if seqErr != nil {
			return 0, 0, seqErr
		}
		data := EventCosmos{
			AuthEventNIDs:    mapFromEventNIDArray(authEventNIDs),
			Depth:            depth,
			EventId:          eventID,
			EventNID:         eventNIDSeq,
			EventStateKeyNID: int64(eventStateKeyNID),
			EventTypeNID:     int64(eventTypeNID),
			IsRejected:       isRejected,
			ReferenceSha256:  referenceSHA256,
			RoomNID:          int64(roomNID),
		}

		dbData = &EventCosmosData{
			Id:        cosmosDocId,
			Tn:        s.db.cosmosConfig.TenantName,
			Cn:        dbCollectionName,
			Pk:        pk,
			Timestamp: time.Now().Unix(),
			Event:     data,
		}
	} else {
		modified := !isEventSame(
			dbData.Event,
			roomNID,
			eventTypeNID,
			eventStateKeyNID,
			eventID,
			referenceSHA256,
			authEventNIDs,
			depth,
			isRejected,
		)
		if modified == false {
			return 0, 0, cosmosdbutil.ErrNoRows
		}
		dbData.Event.AuthEventNIDs = mapFromEventNIDArray(authEventNIDs)
		dbData.Event.Depth = depth
		// Dont change the unique keys
		// dbData.Event.EventId = eventID
		// dbData.Event.EventNID = eventNID
		dbData.Event.EventStateKeyNID = int64(eventStateKeyNID)
		dbData.Event.EventTypeNID = int64(eventTypeNID)
		dbData.Event.IsRejected = isRejected
		dbData.Event.ReferenceSha256 = referenceSHA256
		dbData.Event.RoomNID = int64(roomNID)

		dbData.Timestamp = time.Now().Unix()
	}

	// ON CONFLICT DO NOTHING; - Do Upsert
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	if err != nil {
		return 0, 0, err
	}

	eventNID = dbData.Event.EventNID
	return types.EventNID(eventNID), 0, err
}

func (s *eventStatements) SelectEvent(
	ctx context.Context, txn *sql.Tx, eventID string,
) (types.EventNID, types.StateSnapshotNID, error) {

	// "SELECT event_nid, state_snapshot_nid FROM roomserver_events WHERE event_id = $1"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	docId := eventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	var response, err = getEvent(s, ctx, pk, cosmosDocId)
	if err != nil {
		return 0, 0, err
	}

	var event = response.Event

	return types.EventNID(event.EventNID), types.StateSnapshotNID(event.StateSnapshotNID), err
}

// bulkSelectStateEventByID lookups a list of state events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError
func (s *eventStatements) BulkSelectStateEventByID(
	ctx context.Context, eventIDs []string,
) ([]types.StateEntry, error) {
	if len(eventIDs) == 0 {
		return make([]types.StateEntry, len(eventIDs)), nil
	}

	// "SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	// " WHERE event_id IN ($1)" +
	// " ORDER BY event_type_nid, event_state_key_nid ASC"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectStateEventByIDStmt, params)

	if err != nil {
		return nil, err
	}

	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(response))
	i := 0
	for _, item := range response {
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(item.Event.EventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(item.Event.EventStateKeyNID)
		result.EventNID = types.EventNID(item.Event.EventNID)
		i++
	}
	if i != len(eventIDs) {
		// If there are fewer rows returned than IDs then we were asked to lookup event IDs we don't have.
		// We don't know which ones were missing because we don't return the string IDs in the query.
		// However it should be possible debug this by replaying queries or entries from the input kafka logs.
		// If this turns out to be impossible and we do need the debug information here, it would be better
		// to do it as a separate query rather than slowing down/complicating the internal case.
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: state event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, err
}

// bulkSelectStateAtEventByID lookups the state at a list of events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError.
// If we do not have the state for any of the requested events it returns a types.MissingEventError.
func (s *eventStatements) BulkSelectStateAtEventByID(
	ctx context.Context, eventIDs []string,
) ([]types.StateAtEvent, error) {
	if len(eventIDs) == 0 {
		return make([]types.StateAtEvent, len(eventIDs)), nil
	}

	// "SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, is_rejected FROM roomserver_events" +
	// " WHERE event_id IN ($1)"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectStateAtEventByIDStmt, params)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateAtEvent, len(eventIDs))
	i := 0
	for _, item := range response {
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(item.Event.EventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(item.Event.EventStateKeyNID)
		result.EventNID = types.EventNID(item.Event.EventNID)
		result.BeforeStateSnapshotNID = types.StateSnapshotNID(item.Event.StateSnapshotNID)
		result.IsRejected = item.Event.IsRejected
		if result.BeforeStateSnapshotNID == 0 {
			return nil, types.MissingEventError(
				fmt.Sprintf("storage: missing state for event NID %d", result.EventNID),
			)
		}
		i++
	}
	if i != len(eventIDs) {
		return nil, types.MissingEventError(
			fmt.Sprintf("storage: event IDs missing from the database (%d != %d)", i, len(eventIDs)),
		)
	}
	return results, err
}

func (s *eventStatements) UpdateEventState(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID, stateNID types.StateSnapshotNID,
) error {

	// "UPDATE roomserver_events SET state_snapshot_nid = $1 WHERE event_nid = $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNID,
	}

	response, err := queryEvent(s, ctx, s.updateEventStateStmt, params)

	if err != nil {
		return err
	}

	item := response[0]
	item.Event.StateSnapshotNID = int64(stateNID)

	var _, exReplace = setEvent(s, ctx, item)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}

func (s *eventStatements) SelectEventSentToOutput(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (sentToOutput bool, err error) {

	// 	"SELECT sent_to_output FROM roomserver_events WHERE event_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNID,
	}

	response, err := queryEvent(s, ctx, s.selectEventSentToOutputStmt, params)

	if err != nil {
		return false, err
	}

	item := response[0]
	sentToOutput = item.Event.SentToOutput
	return
}

func (s *eventStatements) UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error {

	// "UPDATE roomserver_events SET sent_to_output = TRUE WHERE event_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNID,
	}

	response, err := queryEvent(s, ctx, s.updateEventSentToOutputStmt, params)

	if err != nil {
		return err
	}

	item := response[0]
	item.Event.SentToOutput = true

	var _, exReplace = setEvent(s, ctx, item)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}

func (s *eventStatements) SelectEventID(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (eventID string, err error) {

	// "SELECT event_id FROM roomserver_events WHERE event_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNID,
	}

	response, err := queryEvent(s, ctx, s.selectEventIDStmt, params)

	if err != nil {
		return "", err
	}

	item := response[0]
	eventNID = types.EventNID(item.Event.EventNID)
	return
}

func (s *eventStatements) BulkSelectStateAtEventAndReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]types.StateAtEventAndReference, error) {
	if len(eventNIDs) == 0 {
		return make([]types.StateAtEventAndReference, len(eventNIDs)), nil
	}

	// "SELECT event_type_nid, event_state_key_nid, event_nid, state_snapshot_nid, event_id, reference_sha256" +
	// " FROM roomserver_events WHERE event_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectStateAtEventAndReferenceStmt, params)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateAtEventAndReference, len(response))
	i := 0
	for _, item := range response {
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(item.Event.EventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(item.Event.EventStateKeyNID)
		result.EventNID = types.EventNID(item.Event.EventNID)
		result.BeforeStateSnapshotNID = types.StateSnapshotNID(item.Event.StateSnapshotNID)
		result.EventID = item.Event.EventId
		result.EventSHA256 = item.Event.ReferenceSha256
		i++
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

func (s *eventStatements) BulkSelectEventReference(
	ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID,
) ([]gomatrixserverlib.EventReference, error) {
	if len(eventNIDs) == 0 {
		return make([]gomatrixserverlib.EventReference, len(eventNIDs)), nil
	}
	// "SELECT event_id, reference_sha256 FROM roomserver_events WHERE event_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectEventReferenceStmt, params)

	if err != nil {
		return nil, err
	}

	results := make([]gomatrixserverlib.EventReference, len(eventNIDs))
	i := 0
	for _, item := range response {
		result := &results[i]
		result.EventID = item.Event.EventId
		result.EventSHA256 = item.Event.ReferenceSha256
		i++
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// bulkSelectEventID returns a map from numeric event ID to string event ID.
func (s *eventStatements) BulkSelectEventID(ctx context.Context, eventNIDs []types.EventNID) (map[types.EventNID]string, error) {
	results := make(map[types.EventNID]string, len(eventNIDs))
	if len(eventNIDs) == 0 {
		return results, nil
	}

	// "SELECT event_nid, event_id FROM roomserver_events WHERE event_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectEventIDStmt, params)

	if err != nil {
		return nil, err
	}

	i := 0
	for _, item := range response {
		eventNID := item.Event.EventNID
		eventID := item.Event.EventId
		results[types.EventNID(eventNID)] = eventID
		i++
	}
	if i != len(eventNIDs) {
		return nil, fmt.Errorf("storage: event NIDs missing from the database (%d != %d)", i, len(eventNIDs))
	}
	return results, nil
}

// bulkSelectEventNIDs returns a map from string event ID to numeric event ID.
// If an event ID is not in the database then it is omitted from the map.
func (s *eventStatements) BulkSelectEventNID(ctx context.Context, eventIDs []string) (map[string]types.EventNID, error) {
	if len(eventIDs) == 0 {
		return make(map[string]types.EventNID, len(eventIDs)), nil
	}
	// "SELECT event_id, event_nid FROM roomserver_events WHERE event_id IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventIDs,
	}

	response, err := queryEvent(s, ctx, s.bulkSelectEventNIDStmt, params)

	if err != nil {
		return nil, err
	}

	results := make(map[string]types.EventNID, len(eventIDs))
	for _, item := range response {
		eventID := item.Event.EventId
		eventNID := item.Event.EventNID
		results[eventID] = types.EventNID(eventNID)
	}
	return results, nil
}

func (s *eventStatements) SelectMaxEventDepth(ctx context.Context, txn *sql.Tx, eventNIDs []types.EventNID) (int64, error) {
	if len(eventNIDs) == 0 {
		return 0, nil
	}

	// "SELECT COALESCE(MAX(depth) + 1, 0) FROM roomserver_events WHERE event_nid IN ($1)"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var response []EventCosmosMaxDepth
	params := map[string]interface{}{
		"@x1": s.db.cosmosConfig.TenantName,
		"@x2": dbCollectionName,
		"@x3": eventNIDs,
	}

	var optionsQry = cosmosdbapi.GetQueryAllPartitionsDocumentsOptions()
	var query = cosmosdbapi.GetQuery(selectMaxEventDepthSQL, params)
	var _, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&response,
		optionsQry)

	if err != nil {
		return 0, fmt.Errorf("sqlutil.TxStmt.QueryRowContext: %w", err)
	}
	return response[0].Max, nil
}

func (s *eventStatements) SelectRoomNIDsForEventNIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]types.RoomNID, error) {
	if len(eventNIDs) == 0 {
		return make(map[types.EventNID]types.RoomNID), nil
	}

	// "SELECT event_nid, room_nid FROM roomserver_events WHERE event_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventNIDs,
	}

	response, err := queryEvent(s, ctx, selectRoomNIDsForEventNIDsSQL, params)

	if err != nil {
		return nil, err
	}

	result := make(map[types.EventNID]types.RoomNID)
	for _, item := range response {
		roomNID := types.RoomNID(item.Event.RoomNID)
		eventNID := types.EventNID(item.Event.EventNID)
		result[eventNID] = roomNID
	}
	return result, nil
}
