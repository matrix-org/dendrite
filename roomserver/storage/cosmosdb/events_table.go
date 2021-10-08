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
	"sort"

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

type eventCosmos struct {
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

type eventCosmosMaxDepth struct {
	Max int64 `json:"maxdepth"`
}

type eventCosmosData struct {
	cosmosdbapi.CosmosDocument
	Event eventCosmos `json:"mx_roomserver_event"`
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

// "SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
// " WHERE event_nid IN ($1)"
// // Rest of query is built by BulkSelectStateEventByNID
const bulkSelectStateEventByNIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid) "
	// Rest of query is built by BulkSelectStateEventByNID

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
	"select MAX(c.mx_roomserver_event.depth) maxdepth from c where c._cn = @x1 " +
	" and ARRAY_CONTAINS(@x2, c.mx_roomserver_event.event_nid)"

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

func (s *eventStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *eventStatements) getPartitionKey() string {
	//No easy PK, so just use the collection
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
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

func getEvent(s *eventStatements, ctx context.Context, pk string, docId string) (*eventCosmosData, error) {
	response := eventCosmosData{}
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
	event eventCosmos,
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
	docId := eventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, errGet := getEvent(s, ctx, s.getPartitionKey(), cosmosDocId)

	// ON CONFLICT DO NOTHING;
	//     event_nid INTEGER PRIMARY KEY AUTOINCREMENT,
	var eventNID int64
	if errGet == cosmosdbutil.ErrNoRows {
		eventNIDSeq, seqErr := GetNextEventNID(s, ctx)
		if seqErr != nil {
			return 0, 0, seqErr
		}
		data := eventCosmos{
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

		dbData = &eventCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			Event:          data,
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

		dbData.SetUpdateTime()
	}

	// ON CONFLICT DO NOTHING; - Do Upsert
	err := cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)

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
	docId := eventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, err = getEvent(s, ctx, s.getPartitionKey(), cosmosDocId)
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectStateEventByIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(rows))
	i := 0
	for _, item := range rows {
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

// bulkSelectStateEventByID lookups a list of state events by event ID.
// If any of the requested events are missing from the database it returns a types.MissingEventError
func (s *eventStatements) BulkSelectStateEventByNID(
	ctx context.Context, eventNIDs []types.EventNID,
	stateKeyTuples []types.StateKeyTuple,
) ([]types.StateEntry, error) {
	// "SELECT event_type_nid, event_state_key_nid, event_nid FROM roomserver_events" +
	// " WHERE event_nid IN ($1)"
	// // Rest of query is built by BulkSelectStateEventByNID
	tuples := stateKeyTupleSorter(stateKeyTuples)
	sort.Sort(tuples)
	eventTypeNIDArray, eventStateKeyNIDArray := tuples.typesAndStateKeysAsArrays()
	// params := make([]interface{}, 0, len(eventNIDs)+len(eventTypeNIDArray)+len(eventStateKeyNIDArray))
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}
	// selectOrig := strings.Replace(bulkSelectStateEventByNIDSQL, "($1)", sqlutil.QueryVariadic(len(eventNIDs)), 1)
	selectOrig := bulkSelectStateEventByNIDSQL
	// for _, v := range eventNIDs {
	// 	params = append(params, v)
	// }
	if len(eventTypeNIDArray) > 0 {
		// selectOrig += " AND event_type_nid IN " + sqlutil.QueryVariadicOffset(len(eventTypeNIDArray), len(params))
		selectOrig += " and ARRAY_CONTAINS(@x3, c.mx_roomserver_event.event_type_nid) "
		// for _, v := range eventTypeNIDArray {
		// 	params = append(params, v)
		// }
		params["@x3"] = eventTypeNIDArray
	}
	if len(eventStateKeyNIDArray) > 0 {
		// selectOrig += " AND event_state_key_nid IN " + sqlutil.QueryVariadicOffset(len(eventStateKeyNIDArray), len(params))
		selectOrig += " and ARRAY_CONTAINS(@x4, c.mx_roomserver_event.event_state_key_nid) "
		// for _, v := range eventStateKeyNIDArray {
		// 	params = append(params, v)
		// }
		params["@x4"] = eventStateKeyNIDArray
	}
	// selectOrig += " ORDER BY event_type_nid, event_state_key_nid ASC"
	//No Composite Index so just order by the 1st one
	selectOrig += " order by c.mx_roomserver_event.event_type_nid asc "
	// selectStmt, err := s.db.Prepare(selectOrig)
	// if err != nil {
	// 	return nil, fmt.Errorf("s.db.Prepare: %w", err)
	// }
	// rows, err := selectStmt.QueryContext(ctx, params...)

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectOrig, params, &rows)

	if err != nil {
		return nil, fmt.Errorf("selectStmt.QueryContext: %w", err)
	}
	// defer internal.CloseAndLogIfError(ctx, rows, "bulkSelectStateEventByID: rows.close() failed")
	// We know that we will only get as many results as event IDs
	// because of the unique constraint on event IDs.
	// So we can allocate an array of the correct size now.
	// We might get fewer results than IDs so we adjust the length of the slice before returning it.
	results := make([]types.StateEntry, len(eventNIDs))
	i := 0
	// for ; rows.Next(); i++ {
	for _, item := range rows {
		result := &results[i]
		result.EventTypeNID = types.EventTypeNID(item.Event.EventTypeNID)
		result.EventStateKeyNID = types.EventStateKeyNID(item.Event.EventStateKeyNID)
		result.EventNID = types.EventNID(item.Event.EventNID)
		// if err = rows.Scan(
		// 	&result.EventTypeNID,
		// 	&result.EventStateKeyNID,
		// 	&result.EventNID,
		// ); err != nil {
		// 	return nil, err
		// }
		i++
	}
	return results[:i], err
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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectStateAtEventByIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateAtEvent, len(eventIDs))
	i := 0
	for _, item := range rows {
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNID,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.updateEventStateStmt, params, &rows)

	if err != nil {
		return err
	}

	item := rows[0]
	item.SetUpdateTime()
	item.Event.StateSnapshotNID = int64(stateNID)

	_, exReplace := cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}

func (s *eventStatements) SelectEventSentToOutput(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (sentToOutput bool, err error) {

	// 	"SELECT sent_to_output FROM roomserver_events WHERE event_nid = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNID,
	}

	var rows []eventCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectEventSentToOutputStmt, params, &rows)

	if err != nil {
		return false, err
	}

	item := rows[0]
	sentToOutput = item.Event.SentToOutput
	return
}

func (s *eventStatements) UpdateEventSentToOutput(ctx context.Context, txn *sql.Tx, eventNID types.EventNID) error {

	// "UPDATE roomserver_events SET sent_to_output = TRUE WHERE event_nid = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNID,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.updateEventSentToOutputStmt, params, &rows)

	if err != nil {
		return err
	}

	item := rows[0]
	item.SetUpdateTime()
	item.Event.SentToOutput = true

	_, exReplace := cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
	if exReplace != nil {
		return exReplace
	}
	return exReplace
}

func (s *eventStatements) SelectEventID(
	ctx context.Context, txn *sql.Tx, eventNID types.EventNID,
) (eventID string, err error) {

	// "SELECT event_id FROM roomserver_events WHERE event_nid = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNID,
	}

	var rows []eventCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectEventIDStmt, params, &rows)

	if err != nil {
		return "", err
	}

	item := rows[0]
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectStateAtEventAndReferenceStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	results := make([]types.StateAtEventAndReference, len(rows))
	i := 0
	for _, item := range rows {
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectEventReferenceStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	results := make([]gomatrixserverlib.EventReference, len(eventNIDs))
	i := 0
	for _, item := range rows {
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectEventIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	i := 0
	for _, item := range rows {
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.bulkSelectEventNIDStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	results := make(map[string]types.EventNID, len(eventIDs))
	for _, item := range rows {
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
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventCosmosMaxDepth
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(),
		selectMaxEventDepthSQL, params, &rows)

	if err != nil {
		return 0, fmt.Errorf("sqlutil.TxStmt.QueryRowContext: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	result := rows[0].Max
	if result == 0 {
		return 0, nil
	}
	return result + 1, nil
}

func (s *eventStatements) SelectRoomNIDsForEventNIDs(
	ctx context.Context, eventNIDs []types.EventNID,
) (map[types.EventNID]types.RoomNID, error) {
	if len(eventNIDs) == 0 {
		return make(map[types.EventNID]types.RoomNID), nil
	}

	// "SELECT event_nid, room_nid FROM roomserver_events WHERE event_nid IN ($1)"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventNIDs,
	}

	var rows []eventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectRoomNIDsForEventNIDsSQL, params, &rows)

	if err != nil {
		return nil, err
	}

	result := make(map[types.EventNID]types.RoomNID)
	for _, item := range rows {
		roomNID := types.RoomNID(item.Event.RoomNID)
		eventNID := types.EventNID(item.Event.EventNID)
		result[eventNID] = roomNID
	}
	return result, nil
}
