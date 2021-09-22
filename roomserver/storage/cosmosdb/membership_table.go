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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"
	"github.com/matrix-org/gomatrixserverlib"

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

type membershipCosmos struct {
	RoomNID       int64 `json:"room_nid"`
	TargetNID     int64 `json:"target_nid"`
	SenderNID     int64 `json:"sender_nid"`
	MembershipNID int64 `json:"membership_nid"`
	EventNID      int64 `json:"event_nid"`
	TargetLocal   bool  `json:"target_local"`
	Forgotten     bool  `json:"forgotten"`
}

type membershipCosmosData struct {
	cosmosdbapi.CosmosDocument
	Membership membershipCosmos `json:"mx_roomserver_membership"`
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

// selectLocalServerInRoomSQL is an optimised case for checking if we, the local server,
// are in the room by using the target_local column of the membership table. Normally when
// we want to know if a server is in a room, we have to unmarshal the entire room state which
// is expensive. The presence of a single row from this query suggests we're still in the
// room, no rows returned suggests we aren't.
// "SELECT room_nid FROM roomserver_membership WHERE target_local = 1 AND membership_nid = $1 AND room_nid = $2 LIMIT 1"
const selectLocalServerInRoomSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.target_local = 1" +
	" and c.mx_roomserver_membership.membership_nid = @x2" +
	" and c.mx_roomserver_membership.room_nid = @x3"

// selectServerMembersInRoomSQL is an optimised case for checking for server members in a room.
// The JOIN is significantly leaner than the previous case of looking up event NIDs and reading the
// membership events from the database, as the JOIN query amounts to little more than two index
// scans which are very fast. The presence of a single row from this query suggests the server is
// in the room, no rows returned suggests they aren't.
// "SELECT room_nid FROM roomserver_membership" +
// " JOIN roomserver_event_state_keys ON roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
// " WHERE membership_nid = $1 AND room_nid = $2 AND event_state_key LIKE '%:' || $3 LIMIT 1"
const selectServerInRoomSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	" and c.mx_roomserver_membership.membership_nid = @x2" +
	" and c.mx_roomserver_membership.room_nid = @x3" +
	" and contains(c.mx_roomserver_membership.target_nid, @x4) "

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
	selectLocalServerInRoomStmt string
	selectServerInRoomStmt      string
	// updateMembershipForgetRoomStmt                  *sql.Stmt
	tableName string
}

func (s *membershipStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *membershipStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func (s *membershipStatements) getCollectionEventStateKeys() string {
	return "roomserver_event_state_keys"
}

func (s *membershipStatements) getPartitionKeyEventStateKeys() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionEventStateKeys())
}

func getMembership(s *membershipStatements, ctx context.Context, pk string, docId string) (*membershipCosmosData, error) {
	response := membershipCosmosData{}
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

func setMembership(s *membershipStatements, ctx context.Context, membership membershipCosmosData) (*membershipCosmosData, error) {
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
	s.selectLocalServerInRoomStmt = selectLocalServerInRoomSQL
	s.selectServerInRoomStmt = selectServerInRoomSQL
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

	// 		UNIQUE (room_nid, target_nid)
	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	// " ON CONFLICT DO NOTHING"
	exists, _ := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)
	if exists != nil {
		exists.Membership.RoomNID = int64(roomNID)
		exists.Membership.TargetNID = int64(targetUserNID)
		exists.Membership.TargetLocal = localTarget
		exists.SetUpdateTime()
		_, errSet := setMembership(s, ctx, *exists)
		return errSet
	}

	data := membershipCosmos{
		EventNID:      0,
		Forgotten:     false,
		MembershipNID: 1,
		RoomNID:       int64(roomNID),
		SenderNID:     0,
		TargetLocal:   localTarget,
		TargetNID:     int64(targetUserNID),
	}

	var dbData = membershipCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		Membership:     data,
	}

	// " ON CONFLICT DO NOTHING"
	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
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

	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	response, err := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)
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

	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	response, err := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
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

	var rows []membershipCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
		eventNIDs = append(eventNIDs, types.EventNID(item.Membership.EventNID))
	}
	return
}

func (s *membershipStatements) SelectMembershipsFromRoomAndMembership(
	ctx context.Context,
	roomNID types.RoomNID, membership tables.MembershipState, localOnly bool,
) (eventNIDs []types.EventNID, err error) {
	var stmt string

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
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
	var rows []membershipCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), stmt, params, &rows)

	if err != nil {
		return nil, err
	}

	for _, item := range rows {
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

	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	dbData, err := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)

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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": membershipState,
		"@x3": userID,
	}
	var rows []membershipCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectRoomsWithMembershipStmt, params, &rows)

	if err != nil {
		return nil, err
	}
	var roomNIDs []types.RoomNID
	for _, item := range rows {
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomNIDs,
	}

	var rows []MembershipJoinedCountCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectJoinedUsersSetForRoomsSQL, params, &rows)

	if err != nil {
		return nil, err
	}

	result := make(map[types.EventStateKeyNID]int)
	for _, item := range rows {
		userID := types.EventStateKeyNID(item.TargetNID)
		count := item.RoomCount
		result[userID] = count
	}
	return result, nil
}

func (s *membershipStatements) SelectLocalServerInRoom(ctx context.Context, roomNID types.RoomNID) (bool, error) {
	// "SELECT room_nid FROM roomserver_membership WHERE target_local = 1 AND membership_nid = $1 AND room_nid = $2 LIMIT 1"

	var nid types.RoomNID
	// err := s.selectLocalServerInRoomStmt.QueryRowContext(ctx, tables.MembershipStateJoin, roomNID).Scan(&nid)
	//
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": tables.MembershipStateJoin,
		"@x3": roomNID,
	}
	var rows []membershipCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectLocalServerInRoomStmt, params, &rows)

	if len(rows) == 0 {
		if err == cosmosdbutil.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	nid = types.RoomNID(rows[0].Membership.RoomNID)

	found := nid > 0
	return found, nil
}

func (s *membershipStatements) SelectServerInRoom(ctx context.Context, roomNID types.RoomNID, serverName gomatrixserverlib.ServerName) (bool, error) {
	var nid types.RoomNID
	// "SELECT room_nid FROM roomserver_membership" +
	// " JOIN roomserver_event_state_keys ON roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	// " WHERE membership_nid = $1 AND room_nid = $2 AND event_state_key LIKE '%:' || $3 LIMIT 1"

	//First get the JOIN table
	// SELECT event_state_key_nid FROM roomserver_event_state_keys
	// WHERE event_state_key LIKE '%:' || $3 LIMIT 1
	selectEventStateKeyNIDSQL := "" +
		"select * from c where c._cn = @x1 " +
		"and (endswith(c.mx_roomserver_event_state_keys.event_state_key, \":\") or c.mx_roomserver_event_state_keys.event_state_key = @x2) "

	params := map[string]interface{}{
		"@x1": s.getCollectionEventStateKeys(),
		"@x2": serverName,
	}

	var rowsEventsStateKeys []eventStateKeysCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKeyEventStateKeys(), selectEventStateKeyNIDSQL, params, &rowsEventsStateKeys)

	eventStateKeyNids := []int64{}
	for _, item := range rowsEventsStateKeys {
		eventStateKeyNids = append(eventStateKeyNids, item.EventStateKeys.EventStateKeyNID)
	}

	//Now do the JOIN
	// "SELECT room_nid FROM roomserver_membership" +
	// " JOIN roomserver_event_state_keys ON roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	// " WHERE membership_nid = $1 AND room_nid = $2 AND event_state_key LIKE '%:' || $3 LIMIT 1"

	// err := s.selectServerInRoomStmt.QueryRowContext(ctx, tables.MembershipStateJoin, roomNID, serverName).Scan(&nid)
	params = map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": tables.MembershipStateJoin,
		"@x3": roomNID,
		"@x4": eventStateKeyNids,
	}
	var rows []membershipCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectServerInRoomStmt, params, &rows)

	if len(rows) == 0 {
		if err == cosmosdbutil.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	nid = types.RoomNID(rows[0].Membership.RoomNID)
	return roomNID == nid, nil
}

func (s *membershipStatements) SelectKnownUsers(ctx context.Context, userID types.EventStateKeyNID, searchString string, limit int) ([]string, error) {

	// "  SELECT DISTINCT room_nid FROM roomserver_membership WHERE target_nid=$1 AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) +
	// ") AND membership_nid = " + fmt.Sprintf("%d", tables.MembershipStateJoin) + " AND event_state_key LIKE $2 LIMIT $3"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": userID,
		"@x3": searchString,
		"@x4": limit,
	}

	var rowsDistinctRoom []membershipCosmos
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectKnownUsersSQLDistinctRoom, params, &rowsDistinctRoom)

	if err != nil {
		return nil, err
	}

	rooms := []int64{}
	for _, item := range rowsDistinctRoom {
		rooms = append(rooms, item.RoomNID)
	}

	// "SELECT DISTINCT event_state_key FROM roomserver_membership INNER JOIN roomserver_event_state_keys ON " +
	// "roomserver_membership.target_nid = roomserver_event_state_keys.event_state_key_nid" +
	// " WHERE room_nid IN (" +

	params = map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": rooms,
	}

	var rows []membershipCosmos
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectKnownUsersSQLRooms, params, &rows)

	if err != nil {
		return nil, err
	}

	targetNIDs := []int64{}
	for _, item := range rows {
		targetNIDs = append(targetNIDs, item.TargetNID)
	}

	// HACK: Joined table
	params = map[string]interface{}{
		"@x1": s.getCollectionEventStateKeys(),
		"@x2": targetNIDs,
	}

	bulkSelectEventStateKeyStmt := "select * from c where c._cn = @x1 and ARRAY_CONTAINS(@x2, c.mx_roomserver_event_state_keys.event_state_key_nid)"

	var rowsEventStateKeys []eventStateKeysCosmos
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKeyEventStateKeys(), bulkSelectEventStateKeyStmt, params, &rowsEventStateKeys)

	if err != nil {
		return nil, err
	}

	// SELECT DISTINCT event_state_key

	result := []string{}
	for _, item := range rowsEventStateKeys {
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

	docId := fmt.Sprintf("%d_%d", roomNID, targetUserNID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)
	dbData, err := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)

	if err != nil {
		return err
	}

	dbData.Membership.Forgotten = forget

	_, err = setMembership(s, ctx, *dbData)
	return err
}
