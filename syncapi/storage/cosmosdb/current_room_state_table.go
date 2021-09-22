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

	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const currentRoomStateSchema = `
// -- Stores the current room state for every room.
// CREATE TABLE IF NOT EXISTS syncapi_current_room_state (
//     room_id TEXT NOT NULL,
//     event_id TEXT NOT NULL,
//     type TEXT NOT NULL,
//     sender TEXT NOT NULL,
//     contains_url BOOL NOT NULL DEFAULT false,
//     state_key TEXT NOT NULL,
//     headered_event_json TEXT NOT NULL,
//     membership TEXT,
//     added_at BIGINT,
//     UNIQUE (room_id, type, state_key)
// );
// -- for event deletion
// CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
// -- for querying membership states of users
// -- CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
// -- for querying state by event IDs
// CREATE UNIQUE INDEX IF NOT EXISTS syncapi_current_room_state_eventid_idx ON syncapi_current_room_state(event_id);
// `

type currentRoomStateCosmos struct {
	RoomID            string `json:"room_id"`
	EventID           string `json:"event_id"`
	Type              string `json:"type"`
	Sender            string `json:"sender"`
	ContainsUrl       bool   `json:"contains_url"`
	StateKey          string `json:"state_key"`
	HeaderedEventJSON []byte `json:"headered_event_json"`
	Membership        string `json:"membership"`
	AddedAt           int64  `json:"added_at"`
}

type currentRoomStateCosmosData struct {
	cosmosdbapi.CosmosDocument
	CurrentRoomState currentRoomStateCosmos `json:"mx_syncapi_current_room_state"`
}

// const upsertRoomStateSQL = "" +
// 	"INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, headered_event_json, membership, added_at)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)" +
// 	" ON CONFLICT (room_id, type, state_key)" +
// 	" DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, headered_event_json = $7, membership = $8, added_at = $9"

// "DELETE FROM syncapi_current_room_state WHERE event_id = $1"
const deleteRoomStateByEventIDSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_current_room_state.event_id = @x2 "

// TODO: Check the SQL is correct here
// "DELETE FROM syncapi_current_room_state WHERE event_id = $1"
const DeleteRoomStateForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_current_room_state.room_id = @x2 "

// "SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"
const selectRoomIDsWithMembershipSQL = "" +
	"select distinct c.mx_syncapi_current_room_state.room_id from c where c._cn = @x1 " +
	"and c.mx_syncapi_current_room_state.type = \"m.room.member\" " +
	"and c.mx_syncapi_current_room_state.state_key = @x2 " +
	"and c.mx_syncapi_current_room_state.membership = @x3 "

// "SELECT event_id, headered_event_json FROM syncapi_current_room_state WHERE room_id = $1"
// // WHEN, ORDER BY and LIMIT will be added by prepareWithFilter
const selectCurrentStateSQL = "" +
	"select top @x3 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_current_room_state.room_id = @x2 "
	// // WHEN, ORDER BY (and LIMIT) will be added by prepareWithFilter

// "SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"
const selectJoinedUsersSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_current_room_state.type = \"m.room.member\" " +
	"and c.mx_syncapi_current_room_state.membership = \"join\" "

// const selectStateEventSQL = "" +
// 	"SELECT headered_event_json FROM syncapi_current_room_state WHERE room_id = $1 AND type = $2 AND state_key = $3"

// "SELECT event_id, added_at, headered_event_json, 0 AS session_id, false AS exclude_from_sync, '' AS transaction_id" +
// " FROM syncapi_current_room_state WHERE event_id IN ($1)"
const selectEventsWithEventIDsSQL = "" +
	// TODO: The session_id and transaction_id blanks are here because otherwise
	// the rowsToStreamEvents expects there to be exactly six columns. We need to
	// figure out if these really need to be in the DB, and if so, we need a
	// better permanent fix for this. - neilalexander, 2 Jan 2020
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_syncapi_current_room_state.event_id) "

type currentRoomStateStatements struct {
	db                 *SyncServerDatasource
	streamIDStatements *streamIDStatements
	// upsertRoomStateStmt             *sql.Stmt
	deleteRoomStateByEventIDStmt    string
	DeleteRoomStateForRoomStmt      string
	selectRoomIDsWithMembershipStmt string
	selectJoinedUsersStmt           string
	// selectStateEventStmt            *sql.Stmt
	tableName        string
	jsonPropertyName string
}

func (s *currentRoomStateStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *currentRoomStateStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getEvent(s *currentRoomStateStatements, ctx context.Context, pk string, docId string) (*currentRoomStateCosmosData, error) {
	response := currentRoomStateCosmosData{}
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

func deleteCurrentRoomState(s *currentRoomStateStatements, ctx context.Context, dbData currentRoomStateCosmosData) error {
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

func NewCosmosDBCurrentRoomStateTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.CurrentRoomState, error) {
	s := &currentRoomStateStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	s.deleteRoomStateByEventIDStmt = deleteRoomStateByEventIDSQL
	s.DeleteRoomStateForRoomStmt = DeleteRoomStateForRoomSQL
	s.selectRoomIDsWithMembershipStmt = selectRoomIDsWithMembershipSQL
	s.selectJoinedUsersStmt = selectJoinedUsersSQL
	s.tableName = "current_room_states"
	s.jsonPropertyName = "mx_syncapi_current_room_state"
	return s, nil
}

// JoinedMemberLists returns a map of room ID to a list of joined user IDs.
func (s *currentRoomStateStatements) SelectJoinedUsers(
	ctx context.Context,
) (map[string][]string, error) {

	// "SELECT room_id, state_key FROM syncapi_current_room_state WHERE type = 'm.room.member' AND membership = 'join'"

	// rows, err := s.selectJoinedUsersStmt.QueryContext(ctx)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
	}
	var rows []currentRoomStateCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectJoinedUsersStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	for _, item := range rows {
		var roomID string
		var userID string
		roomID = item.CurrentRoomState.RoomID
		userID = item.CurrentRoomState.StateKey //StateKey and Not UserID - See the SQL above
		users := result[roomID]
		users = append(users, userID)
		result[roomID] = users
	}
	return result, nil
}

// SelectRoomIDsWithMembership returns the list of room IDs which have the given user in the given membership state.
func (s *currentRoomStateStatements) SelectRoomIDsWithMembership(
	ctx context.Context,
	txn *sql.Tx,
	userID string,
	membership string, // nolint: unparam
) ([]string, error) {

	// "SELECT DISTINCT room_id FROM syncapi_current_room_state WHERE type = 'm.room.member' AND state_key = $1 AND membership = $2"

	// stmt := sqlutil.TxStmt(txn, s.selectRoomIDsWithMembershipStmt)
	// rows, err := stmt.QueryContext(ctx, userID, membership)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": membership,
	}

	var rows []currentRoomStateCosmos
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectRoomIDsWithMembershipStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	var result []string
	for _, item := range rows {
		var roomID string
		roomID = item.RoomID
		result = append(result, roomID)
	}
	return result, nil
}

// CurrentState returns all the current state events for the given room.
func (s *currentRoomStateStatements) SelectCurrentState(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
	excludeEventIDs []string,
) ([]*gomatrixserverlib.HeaderedEvent, error) {

	// "SELECT event_id, headered_event_json FROM syncapi_current_room_state WHERE room_id = $1"
	// // WHEN, ORDER BY and LIMIT will be added by prepareWithFilter

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": stateFilter.Limit,
	}

	stmt, params := prepareWithFilters(
		s.jsonPropertyName, selectCurrentStateSQL, params,
		stateFilter.Senders, stateFilter.NotSenders,
		stateFilter.Types, stateFilter.NotTypes,
		excludeEventIDs, stateFilter.Limit, FilterOrderNone,
	)
	var rows []currentRoomStateCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), stmt, params, &rows)

	if err != nil {
		return nil, err
	}

	return rowsToEvents(&rows)
}

func (s *currentRoomStateStatements) DeleteRoomStateByEventID(
	ctx context.Context, txn *sql.Tx, eventID string,
) error {

	// "DELETE FROM syncapi_current_room_state WHERE event_id = $1"
	// stmt := sqlutil.TxStmt(txn, s.deleteRoomStateByEventIDStmt)

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventID,
	}
	var rows []currentRoomStateCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deleteRoomStateByEventIDStmt, params, &rows)

	for _, item := range rows {
		err = deleteCurrentRoomState(s, ctx, item)
	}

	return err
}

func (s *currentRoomStateStatements) DeleteRoomStateForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) error {

	// TODO: Check the SQL is correct here
	// "DELETE FROM syncapi_current_room_state WHERE event_id = $1"

	// stmt := sqlutil.TxStmt(txn, s.DeleteRoomStateForRoomStmt)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
	}
	var rows []currentRoomStateCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.DeleteRoomStateForRoomStmt, params, &rows)

	for _, item := range rows {
		err = deleteCurrentRoomState(s, ctx, item)
	}

	return err
}

func (s *currentRoomStateStatements) UpsertRoomState(
	ctx context.Context, txn *sql.Tx,
	event *gomatrixserverlib.HeaderedEvent, membership *string, addedAt types.StreamPosition,
) error {
	// Parse content as JSON and search for an "url" key
	containsURL := false
	var content map[string]interface{}
	if json.Unmarshal(event.Content(), &content) != nil {
		// Set containsURL to true if url is present
		_, containsURL = content["url"]
	}

	headeredJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// "INSERT INTO syncapi_current_room_state (room_id, event_id, type, sender, contains_url, state_key, headered_event_json, membership, added_at)" +
	// " VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)" +
	// " ON CONFLICT (room_id, type, state_key)" +
	// " DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, headered_event_json = $7, membership = $8, added_at = $9"

	// TODO: Not sure how we can enfore these extra unique indexes
	// CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_id_idx ON syncapi_current_room_state(event_id, room_id, type, sender, contains_url);
	// -- for querying membership states of users
	// -- CREATE INDEX IF NOT EXISTS syncapi_membership_idx ON syncapi_current_room_state(type, state_key, membership) WHERE membership IS NOT NULL AND membership != 'leave';
	// -- for querying state by event IDs
	// CREATE UNIQUE INDEX IF NOT EXISTS syncapi_current_room_state_eventid_idx ON syncapi_current_room_state(event_id);

	// upsert state event
	// stmt := sqlutil.TxStmt(txn, s.upsertRoomStateStmt)
	// _, err = stmt.ExecContext(
	// 	ctx,
	// 	event.RoomID(),
	// 	event.EventID(),
	// 	event.Type(),
	// 	event.Sender(),
	// 	containsURL,
	// 	*event.StateKey(),
	// 	headeredJSON,
	// 	membership,
	// 	addedAt,
	// )

	// " ON CONFLICT (room_id, type, state_key)" +
	docId := fmt.Sprintf("%s_%s_%s", event.RoomID(), event.Type(), *event.StateKey())
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	membershipData := ""
	if membership != nil {
		membershipData = *membership
	}

	dbData, _ := getEvent(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		// " DO UPDATE SET event_id = $2, sender=$4, contains_url=$5, headered_event_json = $7, membership = $8, added_at = $9"
		dbData.SetUpdateTime()
		dbData.CurrentRoomState.EventID = event.EventID()
		dbData.CurrentRoomState.Sender = event.Sender()
		dbData.CurrentRoomState.ContainsUrl = containsURL
		dbData.CurrentRoomState.HeaderedEventJSON = headeredJSON
		dbData.CurrentRoomState.Membership = membershipData
		dbData.CurrentRoomState.AddedAt = int64(addedAt)
	} else {
		data := currentRoomStateCosmos{
			RoomID:            event.RoomID(),
			EventID:           event.EventID(),
			Type:              event.Type(),
			Sender:            event.Sender(),
			ContainsUrl:       containsURL,
			StateKey:          *event.StateKey(),
			HeaderedEventJSON: headeredJSON,
			Membership:        membershipData,
			AddedAt:           int64(addedAt),
		}

		dbData = &currentRoomStateCosmosData{
			CosmosDocument:   cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			CurrentRoomState: data,
		}
	}

	// _, err = sqlutil.TxStmt(txn, s.insertAccountDataStmt).ExecContext(ctx, pos, userID, roomID, dataType, pos)
	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func minOfInts(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func (s *currentRoomStateStatements) SelectEventsWithEventIDs(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	// iEventIDs := make([]interface{}, len(eventIDs))
	// for k, v := range eventIDs {
	// 	iEventIDs[k] = v
	// }
	res := make([]types.StreamEvent, 0, len(eventIDs))
	var start int
	for start < len(eventIDs) {
		n := minOfInts(len(eventIDs)-start, 999)
		// "SELECT event_id, added_at, headered_event_json, 0 AS session_id, false AS exclude_from_sync, '' AS transaction_id" +
		// " FROM syncapi_current_room_state WHERE event_id IN ($1)"

		// query := strings.Replace(selectEventsWithEventIDsSQL, "@x2", sql.QueryVariadic(n), 1)

		// rows, err := txn.QueryContext(ctx, query, iEventIDs[start:start+n]...)
		params := map[string]interface{}{
			"@x1": s.getCollectionName(),
			"@x2": eventIDs,
		}

		var rows []currentRoomStateCosmosData
		err := cosmosdbapi.PerformQuery(ctx,
			s.db.connection,
			s.db.cosmosConfig.DatabaseName,
			s.db.cosmosConfig.ContainerName,
			s.getPartitionKey(), s.DeleteRoomStateForRoomStmt, params, &rows)

		if err != nil {
			return nil, err
		}
		start = start + n
		events, err := rowsToStreamEventsFromCurrentRoomState(&rows)
		if err != nil {
			return nil, err
		}
		res = append(res, events...)
	}
	return res, nil
}

// Copied from output_room_events_table
func rowsToStreamEventsFromCurrentRoomState(rows *[]currentRoomStateCosmosData) ([]types.StreamEvent, error) {
	var result []types.StreamEvent
	for _, item := range *rows {
		var (
			eventID         string
			streamPos       types.StreamPosition
			eventBytes      []byte
			excludeFromSync bool
			// Not required for this call, see output_room_events_table
			// sessionID       *int64
			// txnID           *string
			// transactionID   *api.TransactionID
		)
		// if err := rows.Scan(&eventID, &streamPos, &eventBytes, &sessionID, &excludeFromSync, &txnID); err != nil {
		// 	return nil, err
		// }
		// Taken from the SQL above
		eventID = item.CurrentRoomState.EventID
		streamPos = types.StreamPosition(item.CurrentRoomState.AddedAt)

		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}

		// Always null for this use-case
		// if sessionID != nil && txnID != nil {
		// 	transactionID = &api.TransactionID{
		// 		SessionID:     *sessionID,
		// 		TransactionID: *txnID,
		// 	}
		// }

		result = append(result, types.StreamEvent{
			HeaderedEvent:   &ev,
			StreamPosition:  streamPos,
			TransactionID:   nil,
			ExcludeFromSync: excludeFromSync,
		})
	}
	return result, nil
}

func rowsToEvents(rows *[]currentRoomStateCosmosData) ([]*gomatrixserverlib.HeaderedEvent, error) {
	result := []*gomatrixserverlib.HeaderedEvent{}
	for _, item := range *rows {
		var eventID string
		var eventBytes []byte
		eventID = item.CurrentRoomState.EventID
		eventBytes = item.CurrentRoomState.HeaderedEventJSON
		// TODO: Handle redacted events
		var ev gomatrixserverlib.HeaderedEvent
		if err := ev.UnmarshalJSONWithEventID(eventBytes, eventID); err != nil {
			return nil, err
		}
		result = append(result, &ev)
	}
	return result, nil
}

func (s *currentRoomStateStatements) SelectStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {

	// stmt := s.selectStateEventStmt
	var res []byte

	// " ON CONFLICT (room_id, type, state_key)" +
	docId := fmt.Sprintf("%s_%s_%s", roomID, evType, stateKey)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	var response, err = getEvent(s, ctx, s.getPartitionKey(), cosmosDocId)

	// err := stmt.QueryRowContext(ctx, roomID, evType, stateKey).Scan(&res)
	if err == cosmosdbutil.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	res = response.CurrentRoomState.HeaderedEventJSON
	var ev gomatrixserverlib.HeaderedEvent
	if err = json.Unmarshal(res, &ev); err != nil {
		return nil, err
	}
	return &ev, err
}
