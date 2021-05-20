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

// const roomsSchema = `
//   CREATE TABLE IF NOT EXISTS roomserver_rooms (
//     room_nid INTEGER PRIMARY KEY AUTOINCREMENT,
//     room_id TEXT NOT NULL UNIQUE,
//     latest_event_nids TEXT NOT NULL DEFAULT '[]',
//     last_event_sent_nid INTEGER NOT NULL DEFAULT 0,
//     state_snapshot_nid INTEGER NOT NULL DEFAULT 0,
//     room_version TEXT NOT NULL
//   );
// `

type RoomCosmosData struct {
	Id        string     `json:"id"`
	Pk        string     `json:"_pk"`
	Cn        string     `json:"_cn"`
	ETag      string     `json:"_etag"`
	Timestamp int64      `json:"_ts"`
	Room      RoomCosmos `json:"mx_roomserver_room"`
}

type RoomCosmos struct {
	RoomNID          int64   `json:"room_nid"`
	RoomID           string  `json:"room_id"`
	LatestEventNIDs  []int64 `json:"latest_event_nids"`
	LastEventSentNID int64   `json:"last_event_sent_nid"`
	StateSnapshotNID int64   `json:"state_snapshot_nid"`
	RoomVersion      string  `json:"room_version"`
}

// Same as insertEventTypeNIDSQL
// const insertRoomNIDSQL = `
// 	INSERT INTO roomserver_rooms (room_id, room_version) VALUES ($1, $2)
// 	  ON CONFLICT DO NOTHING;
// `

// "SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"
// const selectRoomNIDSQL = "" +
// 	"select * from c where c._cn = @x1 and c.mx_roomserver_room.room_nid = @x1"

// "SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"
const selectLatestEventNIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_roomserver_room.room_nid = @x2"

// "SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"
const selectLatestEventNIDsForUpdateSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_room.room_nid = @x2"

// const updateLatestEventNIDsSQL = "" +
// 	"UPDATE roomserver_rooms SET latest_event_nids = $1, last_event_sent_nid = $2, state_snapshot_nid = $3 WHERE room_nid = $4"

// "SELECT room_nid, room_version FROM roomserver_rooms WHERE room_nid IN ($1)"
const selectRoomVersionsForRoomNIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and  ARRAY_CONTAINS(@x2, c.mx_roomserver_room.room_nid)"

// "SELECT room_version, room_nid, state_snapshot_nid, latest_event_nids FROM roomserver_rooms WHERE room_id = $1"
// const selectRoomInfoSQL = "" +
// 	"select * from c where c._cn = @x1 and c.mx_roomserver_room.room_id = @x2"

// "SELECT room_id FROM roomserver_rooms"
const selectRoomIDsSQL = "" +
	"select * from c where c._cn = @x1"

// 	"SELECT room_id FROM roomserver_rooms WHERE room_nid IN ($1)"
const bulkSelectRoomIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and ARRAY_CONTAINS(@x2, c.mx_roomserver_room.room_nid)"

// 	"SELECT room_nid FROM roomserver_rooms WHERE room_id IN ($1)"
const bulkSelectRoomNIDsSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_room.room_nid)"

type roomStatements struct {
	db *Database
	// insertRoomNIDStmt                  *sql.Stmt
	// selectRoomNIDStmt string
	selectLatestEventNIDsStmt          string
	selectLatestEventNIDsForUpdateStmt string
	updateLatestEventNIDsStmt          string
	selectRoomVersionForRoomNIDStmt    string
	// selectRoomInfoStmt                 *sql.Stmt
	selectRoomIDsStmt string
	tableName         string
}

func NewCosmosDBRoomsTable(db *Database) (tables.Rooms, error) {
	s := &roomStatements{
		db: db,
	}
	// return s, shared.StatementList{
	// {&s.insertRoomNIDStmt, insertRoomNIDSQL},
	// {&s.selectRoomNIDStmt, selectRoomNIDSQL},
	s.selectLatestEventNIDsStmt = selectLatestEventNIDsSQL
	s.selectLatestEventNIDsForUpdateStmt = selectLatestEventNIDsForUpdateSQL
	// {&s.updateLatestEventNIDsStmt, updateLatestEventNIDsSQL},
	//{&s.selectRoomVersionForRoomNIDsStmt, selectRoomVersionForRoomNIDsSQL},
	// {&s.selectRoomInfoStmt, selectRoomInfoSQL},
	s.selectRoomIDsStmt = selectRoomIDsSQL
	// }.Prepare(db)
	s.tableName = "rooms"
	return s, nil
}

func mapToRoomEventNIDArray(eventNIDs []int64) []types.EventNID {
	result := []types.EventNID{}
	for i := 0; i < len(eventNIDs); i++ {
		result = append(result, types.EventNID(eventNIDs[i]))
	}
	return result
}

func queryRoom(s *roomStatements, ctx context.Context, qry string, params map[string]interface{}) ([]RoomCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	var response []RoomCosmosData

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

func getRoom(s *roomStatements, ctx context.Context, pk string, docId string) (*RoomCosmosData, error) {
	response := RoomCosmosData{}
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

func setRoom(s *roomStatements, ctx context.Context, pk string, room RoomCosmosData) (*RoomCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(pk, room.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		room.Id,
		&room,
		optionsReplace)
	return &room, ex
}

func (s *roomStatements) SelectRoomIDs(ctx context.Context) ([]string, error) {

	// "SELECT room_id FROM roomserver_rooms"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
	}

	response, err := queryRoom(s, ctx, s.selectRoomIDsStmt, params)

	if err != nil {
		return nil, err
	}

	var roomIDs []string
	for _, item := range response {
		roomIDs = append(roomIDs, item.Room.RoomID)
	}
	return roomIDs, nil
}

func (s *roomStatements) SelectRoomInfo(ctx context.Context, roomID string) (*types.RoomInfo, error) {
	info := types.RoomInfo{}

	// 	"SELECT room_version, room_nid, state_snapshot_nid, latest_event_nids FROM roomserver_rooms WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     room_id TEXT NOT NULL UNIQUE,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	room, err := getRoom(s, ctx, pk, cosmosDocId)

	if err != nil {
		if err == cosmosdbutil.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	info.RoomVersion = gomatrixserverlib.RoomVersion(room.Room.RoomVersion)
	info.RoomNID = types.RoomNID(room.Room.RoomNID)
	info.StateSnapshotNID = types.StateSnapshotNID(room.Room.StateSnapshotNID)
	info.IsStub = len(room.Room.LatestEventNIDs) == 0
	return &info, err
}

func (s *roomStatements) InsertRoomNID(
	ctx context.Context, txn *sql.Tx,
	roomID string, roomVersion gomatrixserverlib.RoomVersion,
) (roomNID types.RoomNID, err error) {

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	// INSERT INTO roomserver_rooms (room_id, room_version) VALUES ($1, $2)
	//   ON CONFLICT DO NOTHING;
	//     room_id TEXT NOT NULL UNIQUE,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)

	dbData, errGet := getRoom(s, ctx, pk, cosmosDocId)

	if errGet == cosmosdbutil.ErrNoRows {
		//     room_nid INTEGER PRIMARY KEY AUTOINCREMENT,
		roomNIDSeq, seqErr := GetNextRoomNID(s, ctx)
		if seqErr != nil {
			return 0, seqErr
		}

		data := RoomCosmos{
			RoomNID:     int64(roomNIDSeq),
			RoomID:      roomID,
			RoomVersion: string(roomVersion),
		}

		dbData = &RoomCosmosData{
			Id:        cosmosDocId,
			Cn:        dbCollectionName,
			Pk:        pk,
			Timestamp: time.Now().Unix(),
			Room:      data,
		}
	}

	// ON CONFLICT DO NOTHING; - Do Upsert
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	if err != nil {
		return 0, fmt.Errorf("s.SelectRoomNID: %w", err)
	}

	roomNID = types.RoomNID(dbData.Room.RoomNID)

	return
}

func (s *roomStatements) SelectRoomNID(
	ctx context.Context, txn *sql.Tx, roomID string,
) (types.RoomNID, error) {

	// "SELECT room_nid FROM roomserver_rooms WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     room_id TEXT NOT NULL UNIQUE,
	docId := roomID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.ContainerName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.ContainerName, dbCollectionName)
	room, err := getRoom(s, ctx, pk, cosmosDocId)

	if err != nil {
		return 0, err
	}

	if room == nil {
		return 0, nil
	}
	return types.RoomNID(room.Room.RoomNID), err
}

func (s *roomStatements) SelectLatestEventNIDs(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.StateSnapshotNID, error) {

	// 	"SELECT latest_event_nids, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNID,
	}

	response, err := queryRoom(s, ctx, s.selectLatestEventNIDsStmt, params)

	if err != nil {
		return nil, 0, err
	}

	// TODO: Check the error handling
	if len(response) == 0 {
		return nil, 0, cosmosdbutil.ErrNoRows
	}

	//Assume 1 per RoomNID
	room := response[0]
	return mapToRoomEventNIDArray(room.Room.LatestEventNIDs), types.StateSnapshotNID(room.Room.StateSnapshotNID), nil
}

func (s *roomStatements) SelectLatestEventsNIDsForUpdate(
	ctx context.Context, txn *sql.Tx, roomNID types.RoomNID,
) ([]types.EventNID, types.EventNID, types.StateSnapshotNID, error) {

	// "SELECT latest_event_nids, last_event_sent_nid, state_snapshot_nid FROM roomserver_rooms WHERE room_nid = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNID,
	}

	response, err := queryRoom(s, ctx, s.selectLatestEventNIDsForUpdateStmt, params)

	if err != nil {
		return nil, 0, 0, err
	}

	// TODO: Check the error handling
	if len(response) == 0 {
		return nil, 0, 0, cosmosdbutil.ErrNoRows
	}

	//Assume 1 per RoomNID
	room := response[0]
	return mapToRoomEventNIDArray(room.Room.LatestEventNIDs), types.EventNID(room.Room.LastEventSentNID), types.StateSnapshotNID(room.Room.StateSnapshotNID), nil
}

func (s *roomStatements) UpdateLatestEventNIDs(
	ctx context.Context,
	txn *sql.Tx,
	roomNID types.RoomNID,
	eventNIDs []types.EventNID,
	lastEventSentNID types.EventNID,
	stateSnapshotNID types.StateSnapshotNID,
) error {

	// "UPDATE roomserver_rooms SET latest_event_nids = $1, last_event_sent_nid = $2, state_snapshot_nid = $3 WHERE room_nid = $4"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNID,
	}

	response, err := queryRoom(s, ctx, s.selectLatestEventNIDsForUpdateStmt, params)

	if err != nil {
		return err
	}

	// TODO: Check the error handling
	if len(response) == 0 {
		return cosmosdbutil.ErrNoRows
	}

	//Assume 1 per RoomNID
	room := response[0]

	room.Room.LatestEventNIDs = mapFromEventNIDArray(eventNIDs)
	room.Room.LastEventSentNID = int64(lastEventSentNID)
	room.Room.StateSnapshotNID = int64(stateSnapshotNID)

	_, err = setRoom(s, ctx, room.Pk, room)
	return err
}

func (s *roomStatements) SelectRoomVersionsForRoomNIDs(
	ctx context.Context, roomNIDs []types.RoomNID,
) (map[types.RoomNID]gomatrixserverlib.RoomVersion, error) {
	if roomNIDs == nil || len(roomNIDs) == 0 {
		return make(map[types.RoomNID]gomatrixserverlib.RoomVersion), nil
	}

	// 	"SELECT room_nid, room_version FROM roomserver_rooms WHERE room_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNIDs,
	}

	response, err := queryRoom(s, ctx, selectRoomVersionsForRoomNIDsSQL, params)

	if err != nil {
		return nil, err
	}

	result := make(map[types.RoomNID]gomatrixserverlib.RoomVersion)
	for _, item := range response {
		result[types.RoomNID(item.Room.RoomNID)] = gomatrixserverlib.RoomVersion(item.Room.RoomVersion)
	}
	return result, nil
}

func (s *roomStatements) BulkSelectRoomIDs(ctx context.Context, roomNIDs []types.RoomNID) ([]string, error) {
	if roomNIDs == nil || len(roomNIDs) == 0 {
		return []string{}, nil
	}

	// "SELECT room_id FROM roomserver_rooms WHERE room_nid IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNIDs,
	}

	response, err := queryRoom(s, ctx, bulkSelectRoomIDsSQL, params)

	if err != nil {
		return nil, err
	}

	var roomIDs []string
	for _, item := range response {
		roomIDs = append(roomIDs, item.Room.RoomID)
	}
	return roomIDs, nil
}

func (s *roomStatements) BulkSelectRoomNIDs(ctx context.Context, roomIDs []string) ([]types.RoomNID, error) {
	if roomIDs == nil || len(roomIDs) == 0 {
		return []types.RoomNID{}, nil
	}

	// "SELECT room_nid FROM roomserver_rooms WHERE room_id IN ($1)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomIDs,
	}

	response, err := queryRoom(s, ctx, bulkSelectRoomNIDsSQL, params)

	if err != nil {
		return nil, err
	}

	var roomNIDs []types.RoomNID
	for _, item := range response {
		roomNIDs = append(roomNIDs, types.RoomNID(item.Room.RoomNID))
	}
	return roomNIDs, nil
}
