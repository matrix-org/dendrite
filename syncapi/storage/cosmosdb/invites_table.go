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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const inviteEventsSchema = `
// CREATE TABLE IF NOT EXISTS syncapi_invite_events (
// 	id INTEGER PRIMARY KEY,
// 	event_id TEXT NOT NULL,
// 	room_id TEXT NOT NULL,
// 	target_user_id TEXT NOT NULL,
// 	headered_event_json TEXT NOT NULL,
// 	deleted BOOL NOT NULL
// );

// CREATE INDEX IF NOT EXISTS syncapi_invites_target_user_id_idx ON syncapi_invite_events (target_user_id, id);
// CREATE INDEX IF NOT EXISTS syncapi_invites_event_id_idx ON syncapi_invite_events (event_id);
// `

type inviteEventCosmos struct {
	ID                int64  `json:"id"`
	EventID           string `json:"event_id"`
	RoomID            string `json:"room_id"`
	TargetUserID      string `json:"target_user_id"`
	HeaderedEventJSON []byte `json:"headered_event_json"`
	Deleted           bool   `json:"deleted"`
}

type inviteEventCosmosMaxNumber struct {
	Max int64 `json:"number"`
}

type inviteEventCosmosData struct {
	cosmosdbapi.CosmosDocument
	InviteEvent inviteEventCosmos `json:"mx_syncapi_invite_event"`
}

// const insertInviteEventSQL = "" +
// 	"INSERT INTO syncapi_invite_events" +
// 	" (id, room_id, event_id, target_user_id, headered_event_json, deleted)" +
// 	" VALUES ($1, $2, $3, $4, $5, false)"

// "UPDATE syncapi_invite_events SET deleted=true, id=$1 WHERE event_id = $2"
const deleteInviteEventSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_invite_event.event_id = @x2 "

// "SELECT room_id, headered_event_json, deleted FROM syncapi_invite_events" +
// " WHERE target_user_id = $1 AND id > $2 AND id <= $3" +
// " ORDER BY id DESC"
const selectInviteEventsInRangeSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_invite_event.target_user_id = @x2 " +
	"and c.mx_syncapi_invite_event.id > @x3 " +
	"and c.mx_syncapi_invite_event.id <= @x4 " +
	"order by c.mx_syncapi_invite_event.id desc "

// "SELECT MAX(id) FROM syncapi_invite_events"
const selectMaxInviteIDSQL = "" +
	"select max(c.mx_syncapi_invite_event.id) from c where c._cn = @x1 " +
	"and c._sid = @x2 "

type inviteEventsStatements struct {
	db                 *SyncServerDatasource
	streamIDStatements *streamIDStatements
	// insertInviteEventStmt         *sql.Stmt
	selectInviteEventsInRangeStmt string
	deleteInviteEventStmt         string
	selectMaxInviteIDStmt         string
	tableName                     string
}

func (s *inviteEventsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *inviteEventsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getInviteEvent(s *inviteEventsStatements, ctx context.Context, pk string, docId string) (*inviteEventCosmosData, error) {
	response := inviteEventCosmosData{}
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

func NewCosmosDBInvitesTable(db *SyncServerDatasource, streamID *streamIDStatements) (tables.Invites, error) {
	s := &inviteEventsStatements{
		db:                 db,
		streamIDStatements: streamID,
	}
	s.selectInviteEventsInRangeStmt = selectInviteEventsInRangeSQL
	s.deleteInviteEventStmt = deleteInviteEventSQL
	s.selectMaxInviteIDStmt = selectMaxInviteIDSQL
	s.tableName = "invite_events"
	return s, nil
}

func (s *inviteEventsStatements) InsertInviteEvent(
	ctx context.Context, txn *sql.Tx, inviteEvent *gomatrixserverlib.HeaderedEvent,
) (streamPos types.StreamPosition, err error) {

	// "INSERT INTO syncapi_invite_events" +
	// " (id, room_id, event_id, target_user_id, headered_event_json, deleted)" +
	// " VALUES ($1, $2, $3, $4, $5, false)"

	streamPos, err = s.streamIDStatements.nextInviteID(ctx, txn)
	if err != nil {
		return
	}

	var headeredJSON []byte
	headeredJSON, err = json.Marshal(inviteEvent)
	if err != nil {
		return
	}

	// stmt := sqlutil.TxStmt(txn, s.insertInviteEventStmt)
	// _, err = stmt.ExecContext(
	// 	ctx,
	// 	streamPos,
	// 	inviteEvent.RoomID(),
	// 	inviteEvent.EventID(),
	// 	*inviteEvent.StateKey(),
	// 	headeredJSON,
	// )
	data := inviteEventCosmos{
		ID:                int64(streamPos),
		RoomID:            inviteEvent.RoomID(),
		EventID:           inviteEvent.EventID(),
		TargetUserID:      *inviteEvent.StateKey(),
		HeaderedEventJSON: headeredJSON,
	}

	// id INTEGER PRIMARY KEY,
	docId := fmt.Sprintf("%d", streamPos)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var dbData = inviteEventCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
		InviteEvent:    data,
	}

	var optionsCreate = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err = cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData,
		optionsCreate)

	return
}

func (s *inviteEventsStatements) DeleteInviteEvent(
	ctx context.Context, txn *sql.Tx, inviteEventID string,
) (types.StreamPosition, error) {
	streamPos, err := s.streamIDStatements.nextInviteID(ctx, txn)
	if err != nil {
		return streamPos, err
	}

	// "UPDATE syncapi_invite_events SET deleted=true, id=$1 WHERE event_id = $2"

	// stmt := sqlutil.TxStmt(txn, s.deleteInviteEventStmt)
	// _, err = stmt.ExecContext(ctx, streamPos, inviteEventID)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": inviteEventID,
	}
	var rows []inviteEventCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deleteInviteEventStmt, params, &rows)

	for _, item := range rows {
		item.SetUpdateTime()
		item.InviteEvent.Deleted = true
		item.InviteEvent.ID = int64(streamPos)
		_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
	}
	return streamPos, err
}

// selectInviteEventsInRange returns a map of room ID to invite event for the
// active invites for the target user ID in the supplied range.
func (s *inviteEventsStatements) SelectInviteEventsInRange(
	ctx context.Context, txn *sql.Tx, targetUserID string, r types.Range,
) (map[string]*gomatrixserverlib.HeaderedEvent, map[string]*gomatrixserverlib.HeaderedEvent, error) {

	// "SELECT room_id, headered_event_json, deleted FROM syncapi_invite_events" +
	// " WHERE target_user_id = $1 AND id > $2 AND id <= $3" +
	// " ORDER BY id DESC"

	// stmt := sqlutil.TxStmt(txn, s.selectInviteEventsInRangeStmt)
	// rows, err := stmt.QueryContext(ctx, targetUserID, r.Low(), r.High())
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": targetUserID,
		"@x3": r.Low(),
		"@x4": r.High(),
	}
	var rows []inviteEventCosmosData
	err := cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectInviteEventsInRangeStmt, params, &rows)

	if err != nil {
		return nil, nil, err
	}
	result := map[string]*gomatrixserverlib.HeaderedEvent{}
	retired := map[string]*gomatrixserverlib.HeaderedEvent{}
	for _, item := range rows {
		var (
			roomID    string
			eventJSON []byte
			deleted   bool
		)
		roomID = item.InviteEvent.RoomID
		eventJSON = item.InviteEvent.HeaderedEventJSON
		deleted = item.InviteEvent.Deleted
		// if err = rows.Scan(&roomID, &eventJSON, &deleted); err != nil {
		// 	return nil, nil, err
		// }

		// if we have seen this room before, it has a higher stream position and hence takes priority
		// because the query is ORDER BY id DESC so drop them
		_, isRetired := retired[roomID]
		_, isInvited := result[roomID]
		if isRetired || isInvited {
			continue
		}

		var event *gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(eventJSON, &event); err != nil {
			return nil, nil, err
		}
		if deleted {
			retired[roomID] = event
		} else {
			result[roomID] = event
		}
	}
	return result, retired, nil
}

func (s *inviteEventsStatements) SelectMaxInviteID(
	ctx context.Context, txn *sql.Tx,
) (id int64, err error) {
	var nullableID sql.NullInt64

	// "SELECT MAX(id) FROM syncapi_invite_events"

	// stmt := sqlutil.TxStmt(txn, s.selectMaxInviteIDStmt)
	// err = stmt.QueryRowContext(ctx).Scan(&nullableID)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": s.db.cosmosConfig.TenantName,
	}

	var rows []inviteEventCosmosMaxNumber
	err = cosmosdbapi.PerformQueryAllPartitions(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.selectMaxInviteIDStmt, params, &rows)

	if len(rows) > 0 {
		nullableID.Int64 = rows[0].Max
	}

	if nullableID.Valid {
		id = nullableID.Int64
	}
	return
}
