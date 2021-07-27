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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
)

// const membershipSchema = `
// 	CREATE TABLE IF NOT EXISTS roomserver_membership (
// 		room_nid INTEGER NOT NULL,
// 		target_nid INTEGER NOT NULL,
// 		sender_nid INTEGER NOT NULL DEFAULT 0,
// 		membership_nid INTEGER NOT NULL DEFAULT 1,
// 		event_nid INTEGER NOT NULL DEFAULT 0,
// 		target_local BOOLEAN NOT NULL DEFAULT false,
// 		forgotten BOOLEAN NOT NULL DEFAULT false,
// 		UNIQUE (room_nid, target_nid)
// 	);
// `

type MembershipCosmos struct {
	RoomNID       int64 `json:"room_nid"`
	TargetNID     int64 `json:"target_nid"`
	SenderNID     int64 `json:"sender_nid"`
	MembershipNID int64 `json:"membership_nid"`
	EventNID      int64 `json:"event_nid"`
	TargetLocal   bool  `json:"target_local"`
	Forgotten     bool  `json:"forgotten"`
}

type MembershipCosmosData struct {
	Id         string           `json:"id"`
	Pk         string           `json:"_pk"`
	Tn         string           `json:"_sid"`
	Cn         string           `json:"_cn"`
	ETag       string           `json:"_etag"`
	Timestamp  int64            `json:"_ts"`
	Membership MembershipCosmos `json:"mx_roomserver_membership"`
}

type MembershipJoinedCountCosmosData struct {
	TargetNID int64 `json:"target_nid"`
	RoomCount int   `json:"room_count"`
}

// "SELECT target_nid, COUNT(room_nid) FROM roomserver_membership WHERE room_nid IN ($1) AND" +
// " membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " and forgotten = false" +
// " GROUP BY target_nid"
var selectJoinedUsersSetForRoomsSQL = "" +
	"select c.mx_roomserver_membership.target_nid, count(c.mx_roomserver_membership.room_id) as room_count from c where c._cn = @x1 " +
	" and ARRAY_CONTAINS(@x2, c.mx_roomserver_membership.room_id)" +
	" and c.mx_roomserver_membership.membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
	" and c.mx_roomserver_membership.forgotten = false" +
	" group by c.mx_roomserver_membership.target_nid"

// Insert a row in to membership table so that it can be locked by the
// SELECT FOR UPDATE
// const insertMembershipSQL = "" +
// 	"INSERT INTO roomserver_membership (room_nid, target_nid, target_local)" +
// 	" VALUES ($1, $2, $3)" +
// 	" ON CONFLICT DO NOTHING"

// const selectMembershipFromRoomAndTargetSQL = "" +
// 	"SELECT membership_nid, event_nid, forgotten FROM roomserver_membership" +
// 	" WHERE room_nid = $1 AND target_nid = $2"

// "SELECT event_nid FROM roomserver_membership" +
// " WHERE room_nid = $1 AND membership_nid = $2 and forgotten = false"
const selectMembershipsFromRoomAndMembershipSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.room_nid = @x2" +
	" and c.mx_roomserver_membership.membership_nid = @x3" +
	" and c.mx_roomserver_membership.forgotten = false"

// "SELECT event_nid FROM roomserver_membership" +
// " WHERE room_nid = $1 AND membership_nid = $2" +
// " AND target_local = true and forgotten = false"
const selectLocalMembershipsFromRoomAndMembershipSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.room_nid = @x2" +
	" and c.mx_roomserver_membership.membership_nid = @x3" +
	" and c.mx_roomserver_membership.target_local = true" +
	" and c.mx_roomserver_membership.forgotten = false"

// "SELECT event_nid FROM roomserver_membership" +
// " WHERE room_nid = $1 and forgotten = false"
const selectMembershipsFromRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.room_nid = @x2" +
	" and c.mx_roomserver_membership.forgotten = false"

// "SELECT event_nid FROM roomserver_membership" +
// " WHERE room_nid = $1" +
// " AND target_local = true and forgotten = false"
const selectLocalMembershipsFromRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.room_nid = @x2" +
	" and c.mx_roomserver_membership.target_local = true" +
	" and c.mx_roomserver_membership.forgotten = false"

// const selectMembershipForUpdateSQL = "" +
// 	"SELECT membership_nid FROM roomserver_membership" +
// 	" WHERE room_nid = $1 AND target_nid = $2"

// const updateMembershipSQL = "" +
// 	"UPDATE roomserver_membership SET sender_nid = $1, membership_nid = $2, event_nid = $3, forgotten = $4" +
// 	" WHERE room_nid = $5 AND target_nid = $6"

// const updateMembershipForgetRoom = "" +
// 	"UPDATE roomserver_membership SET forgotten = $1" +
// 	" WHERE room_nid = $2 AND target_nid = $3"

// "SELECT room_nid FROM roomserver_membership WHERE membership_nid = $1 AND target_nid = $2 and forgotten = false"
const selectRoomsWithMembershipSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.membership_nid = @x2" +
	" and c.mx_roomserver_membership.target_nid = true" +
	" and c.mx_roomserver_membership.forgotten = false"

// selectKnownUsersSQL uses a sub-select statement here to find rooms that the user is
// joined to. Since this information is used to populate the user directory, we will
// only return users that the user would ordinarily be able to see anyway.
// var selectKnownUsersSQL = "" +
// "SELECT DISTINCT event_state_key FROM roomserver_membership INNER JOIN roomserver_event_state_keys ON " +
// "roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
// " WHERE room_nid IN (" +
// "  SELECT DISTINCT room_nid FROM roomserver_membership WHERE target_nid=$1 AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
// ") AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " AND event_state_key LIKE $2 LIMIT $3"

var selectKnownUsersSQLRooms = "" +
	"select * from c where c._cn = @x1 " +
	"and ARRAY_CONTAINS(@x2, c.mx_roomserver_membership.room_id)"

var selectKnownUsersSQLDistinctRoom = "" +
	"select distinct top @x4 c.mx_roomserver_membership.room_nid as room_nid from c where c._cn = @x1 " +
	"and c.mx_roomserver_membership.target_nid = @x2 " +
	"and c.mx_roomserver_membership.membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " " +
	"and contains(c.mx_roomserver_membership.event_state_key, @x3) "

type membershipStatements struct {
	db *Database
	// insertMembershipStmt                            *sql.Stmt
	// selectMembershipForUpdateStmt                   string
	// selectMembershipFromRoomAndTargetStmt           string
	selectMembershipsFromRoomAndMembershipStmt      string
	selectLocalMembershipsFromRoomAndMembershipStmt string
	selectMembershipsFromRoomStmt                   string
	selectLocalMembershipsFromRoomStmt              string
	selectRoomsWithMembershipStmt                   string
	// updateMembershipStmt                            *sql.Stmt
	// selectKnownUsersStmt string
	// updateMembershipForgetRoomStmt                  *sql.Stmt
	tableName string
}

func queryMembership(s *membershipStatements, ctx context.Context, qry string, params map[string]interface{}) ([]MembershipCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []MembershipCosmosData

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

func getMembership(s *membershipStatements, ctx context.Context, pk string, docId string) (*MembershipCosmosData, error) {
	response := MembershipCosmosData{}
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

func setMembership(s *membershipStatements, ctx context.Context, membership MembershipCosmosData) (*MembershipCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(membership.Pk, membership.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		membership.Id,
		&membership,
		optionsReplace)
	return &membership, ex
}

func NewCosmosDBMembershipTable(db *Database) (tables.Membership, error) {
	s := &membershipStatements{
		db: db,
	}

	// return s, shared.StatementList{
	// 	{&s.insertMembershipStmt, insertMembershipSQL},
	// s.selectMembershipForUpdateStmt = selectMembershipForUpdateSQL
	// s.selectMembershipFromRoomAndTargetStmt = selectMembershipFromRoomAndTargetSQL
	s.selectMembershipsFromRoomAndMembershipStmt = selectMembershipsFromRoomAndMembershipSQL
	s.selectLocalMembershipsFromRoomAndMembershipStmt = selectLocalMembershipsFromRoomAndMembershipSQL
	s.selectMembershipsFromRoomStmt = selectMembershipsFromRoomSQL
	s.selectLocalMembershipsFromRoomStmt = selectLocalMembershipsFromRoomSQL
	// {&s.updateMembershipStmt, updateMembershipSQL},
	s.selectRoomsWithMembershipStmt = selectRoomsWithMembershipSQL
	// {&s.selectKnownUsersStmt, selectKnownUsersSQL},
	// {&s.updateMembershipForgetRoomStmt, updateMembershipForgetRoom},
	// }.Prepare(db)

	s.tableName = "memberships"
	return s, nil
}

func (s *membershipStatements) InsertMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	localTarget bool,
) error {

	// "INSERT INTO roomserver_membership (room_nid, target_nid, target_local)" +
	// " VALUES ($1, $2, $3)" +
	// " ON CONFLICT DO NOTHING"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	// 		UNIQUE (room_nid, target_nid)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := MembershipCosmos{
		EventNID:      0,
		Forgotten:     false,
		MembershipNID: 1,
		RoomNID:       int64(roomNID),
		SenderNID:     0,
		TargetLocal:   false,
		TargetNID:     int64(targetUserNID),
	}

	var dbData = MembershipCosmosData{
		Id:         cosmosDocId,
		Tn:         s.db.cosmosConfig.TenantName,
		Cn:         dbCollectionName,
		Pk:         pk,
		Timestamp:  time.Now().Unix(),
		Membership: data,
	}

	// " ON CONFLICT DO NOTHING"
	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *membershipStatements) SelectMembershipForUpdate(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (membership tables.MembershipState, err error) {

	// "SELECT membership_nid FROM roomserver_membership" +
	// " WHERE room_nid = $1 AND target_nid = $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	response, err := getMembership(s, ctx, pk, cosmosDocId)
	if response != nil {
		membership = tables.MembershipState(response.Membership.MembershipNID)
	}
	return
}

func (s *membershipStatements) SelectMembershipFromRoomAndTarget(
	ctx context.Context,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
) (eventNID types.EventNID, membership tables.MembershipState, forgotten bool, err error) {

	// 	"SELECT membership_nid, event_nid, forgotten FROM roomserver_membership" +
	// 	" WHERE room_nid = $1 AND target_nid = $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	response, err := getMembership(s, ctx, pk, cosmosDocId)
	if response != nil {
		eventNID = types.EventNID(response.Membership.EventNID)
		forgotten = response.Membership.Forgotten
		membership = tables.MembershipState(response.Membership.MembershipNID)
	}
	return
}

func (s *membershipStatements) SelectMembershipsFromRoom(
	ctx context.Context,
	roomNID types.RoomNID, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var selectStmt string

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNID,
	}
	if localOnly {
		// "SELECT event_nid FROM roomserver_membership" +
		// " WHERE room_nid = $1" +
		// " AND target_local = true and forgotten = false"
		selectStmt = s.selectLocalMembershipsFromRoomStmt

	} else {
		// "SELECT event_nid FROM roomserver_membership" +
		// " WHERE room_nid = $1 and forgotten = false"
		selectStmt = s.selectMembershipsFromRoomStmt
	}
	response, err := queryMembership(s, ctx, selectStmt, params)
	if err != nil {
		return nil, err
	}

	for _, item := range response {
		eventNIDs = append(eventNIDs, types.EventNID(item.Membership.EventNID))
	}
	return
}

func (s *membershipStatements) SelectMembershipsFromRoomAndMembership(
	ctx context.Context,
	roomNID types.RoomNID, membership tables.MembershipState, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var stmt string

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNID,
		"@x3": membership,
	}
	if localOnly {
		// "SELECT event_nid FROM roomserver_membership" +
		// " WHERE room_nid = $1 AND membership_nid = $2" +
		// " AND target_local = true and forgotten = false"
		stmt = s.selectLocalMembershipsFromRoomAndMembershipStmt
	} else {
		// "SELECT event_nid FROM roomserver_membership" +
		// " WHERE room_nid = $1 AND membership_nid = $2 and forgotten = false"
		stmt = s.selectMembershipsFromRoomAndMembershipStmt
	}
	response, err := queryMembership(s, ctx, stmt, params)
	if err != nil {
		return nil, err
	}

	for _, item := range response {
		eventNIDs = append(eventNIDs, types.EventNID(item.Membership.EventNID))
	}
	return
}

func (s *membershipStatements) UpdateMembership(
	ctx context.Context, txn *sql.Tx,
	roomNID types.RoomNID, targetUserNID types.EventStateKeyNID, senderUserNID types.EventStateKeyNID, membership tables.MembershipState,
	eventNID types.EventNID, forgotten bool,
) error {

	// "UPDATE roomserver_membership SET sender_nid = $1, membership_nid = $2, event_nid = $3, forgotten = $4" +
	// " WHERE room_nid = $5 AND target_nid = $6"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	dbData, err := getMembership(s, ctx, pk, cosmosDocId)

	if err != nil {
		return err
	}

	dbData.Membership.SenderNID = int64(senderUserNID)
	dbData.Membership.MembershipNID = int64(membership)
	dbData.Membership.EventNID = int64(eventNID)
	dbData.Membership.Forgotten = forgotten

	_, err = setMembership(s, ctx, *dbData)
	return err
}

func (s *membershipStatements) SelectRoomsWithMembership(
	ctx context.Context, userID types.EventStateKeyNID, membershipState tables.MembershipState,
) ([]types.RoomNID, error) {

	// "SELECT room_nid FROM roomserver_membership WHERE membership_nid = $1 AND target_nid = $2 and forgotten = false"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": membershipState,
		"@x3": userID,
	}
	response, err := queryMembership(s, ctx, s.selectRoomsWithMembershipStmt, params)
	if err != nil {
		return nil, err
	}
	var roomNIDs []types.RoomNID
	for _, item := range response {
		roomNIDs = append(roomNIDs, types.RoomNID(item.Membership.RoomNID))
	}
	return roomNIDs, nil
}

func (s *membershipStatements) SelectJoinedUsersSetForRooms(ctx context.Context, roomNIDs []types.RoomNID) (map[types.EventStateKeyNID]int, error) {
	iRoomNIDs := make([]interface{}, len(roomNIDs))
	for i, v := range roomNIDs {
		iRoomNIDs[i] = v
	}

	// "SELECT target_nid, COUNT(room_nid) FROM roomserver_membership WHERE room_nid IN ($1) AND" +
	// " membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " and forgotten = false" +
	// " GROUP BY target_nid"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomNIDs,
	}
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []MembershipJoinedCountCosmosData

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectJoinedUsersSetForRoomsSQL, params)
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

	result := make(map[types.EventStateKeyNID]int)
	for _, item := range response {
		userID := types.EventStateKeyNID(item.TargetNID)
		count := item.RoomCount
		result[userID] = count
	}
	return result, nil
}

func (s *membershipStatements) SelectKnownUsers(ctx context.Context, userID types.EventStateKeyNID, searchString string, limit int) ([]string, error) {

	// "  SELECT DISTINCT room_nid FROM roomserver_membership WHERE target_nid=$1 AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
	// ") AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " AND event_state_key LIKE $2 LIMIT $3"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": userID,
		"@x3": searchString,
		"@x4": limit,
	}

	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var responseDistinctRoom []MembershipCosmos

	var optionsQry = cosmosdbapi.GetQueryDocumentsOptions(pk)
	var query = cosmosdbapi.GetQuery(selectKnownUsersSQLDistinctRoom, params) //
	_, err := cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&responseDistinctRoom,
		optionsQry)

	if err != nil {
		return nil, err
	}

	rooms := []int64{}
	for _, item := range responseDistinctRoom {
		rooms = append(rooms, item.RoomNID)
	}

	// "SELECT DISTINCT event_state_key FROM roomserver_membership INNER JOIN roomserver_event_state_keys ON " +
	// "roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	// " WHERE room_nid IN (" +

	params = map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": rooms,
	}

	var responseRooms []MembershipCosmos
	query = cosmosdbapi.GetQuery(selectKnownUsersSQLRooms, params)
	_, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&responseRooms,
		optionsQry)

	if err != nil {
		return nil, err
	}

	targetNIDs := []int64{}
	for _, item := range responseRooms {
		targetNIDs = append(targetNIDs, item.TargetNID)
	}

	// HACK: Joined table
	var dbCollectionNameEventStateKeys = cosmosdbapi.GetCollectionName(s.db.databaseName, "event_state_keys")
	params = map[string]interface{}{
		"@x1": dbCollectionNameEventStateKeys,
		"@x2": targetNIDs,
	}

	bulkSelectEventStateKeyStmt := "select * from c where c._cn = @x1 and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_state_keys.event_state_key_nid)"

	var responseEventStateKeys []EventStateKeysCosmos
	query = cosmosdbapi.GetQuery(bulkSelectEventStateKeyStmt, params)
	_, err = cosmosdbapi.GetClient(s.db.connection).QueryDocuments(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		query,
		&responseEventStateKeys,
		optionsQry)

	if err != nil {
		return nil, err
	}

	// SELECT DISTINCT event_state_key

	result := []string{}
	for _, item := range responseEventStateKeys {
		userID := item.EventStateKey
		result = append(result, userID)
	}
	return result, nil
}

func (s *membershipStatements) UpdateForgetMembership(
	ctx context.Context,
	txn *sql.Tx, roomNID types.RoomNID, targetUserNID types.EventStateKeyNID,
	forget bool,
) error {

	// "UPDATE roomserver_membership SET forgotten = $1" +
	// " WHERE room_nid = $2 AND target_nid = $3"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	dbData, err := getMembership(s, ctx, pk, cosmosDocId)

	if err != nil {
		return err
	}

	dbData.Membership.Forgotten = forget

	_, err = setMembership(s, ctx, *dbData)
	return err
}
