// Copyright 2021 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// The memberships table is designed to track the last time that
// the user was a given state. This allows us to find out the
// most recent time that a user was invited to, joined or left
// a room, either by choice or otherwise. This is important for
// building history visibility.

// const membershipsSchema = `
// CREATE TABLE IF NOT EXISTS syncapi_memberships (
//     -- The 'room_id' key for the state event.
//     room_id TEXT NOT NULL,
//     -- The state event ID
// 	user_id TEXT NOT NULL,
// 	-- The status of the membership
// 	membership TEXT NOT NULL,
// 	-- The event ID that last changed the membership
// 	event_id TEXT NOT NULL,
// 	-- The stream position of the change
// 	stream_pos BIGINT NOT NULL,
// 	-- The topological position of the change in the room
// 	topological_pos BIGINT NOT NULL,
// 	-- Unique index
// 	UNIQUE (room_id, user_id, membership)
// );
// `

type membershipCosmos struct {
	RoomID         string `json:"room_id"`
	UserID         string `json:"user_id"`
	Membership     string `json:"membership"`
	EventID        string `json:"event_id"`
	StreamPos      int64  `json:"stream_pos"`
	TopologicalPos int64  `json:"topological_pos"`
}

type membershipCosmosData struct {
	cosmosdbapi.CosmosDocument
	Membership membershipCosmos `json:"mx_syncapi_membership"`
}

// const upsertMembershipSQL = "" +
// 	"INSERT INTO syncapi_memberships (room_id, user_id, membership, event_id, stream_pos, topological_pos)" +
// 	" VALUES ($1, $2, $3, $4, $5, $6)" +
// 	" ON CONFLICT (room_id, user_id, membership)" +
// 	" DO UPDATE SET event_id = $4, stream_pos = $5, topological_pos = $6"

// "SELECT event_id, stream_pos, topological_pos FROM syncapi_memberships" +
// " WHERE room_id = $1 AND user_id = $2 AND membership IN ($3)" +
// " ORDER BY stream_pos DESC" +
// " LIMIT 1"
const selectMembershipSQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_membership.room_id = @x2 " +
	"and c.mx_syncapi_membership.user_id = @x3 " +
	"and ARRAY_CONTAINS(@x4, c.mx_syncapi_membership.membership) " +
	"order by c.mx_syncapi_membership.stream_pos desc "

type membershipsStatements struct {
	db *SyncServerDatasource
	// upsertMembershipStmt *sql.Stmt
	tableName string
}

func (s *membershipsStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *membershipsStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getMembership(s *membershipsStatements, ctx context.Context, pk string, docId string) (*membershipCosmosData, error) {
	response := membershipCosmosData{}
	err := cosmosdbapi.GetDocumentOrNil(
		s.db.connection,
		s.db.cosmosConfig,
		ctx,
		pk,
		docId,
		&response)

	if response.Id == "" {
		return nil, nil
	}

	return &response, err
}

func NewCosmosDBMembershipsTable(db *SyncServerDatasource) (tables.Memberships, error) {
	s := &membershipsStatements{
		db: db,
	}
	s.tableName = "memberships"
	return s, nil
}

func (s *membershipsStatements) UpsertMembership(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent,
	streamPos, topologicalPos types.StreamPosition,
) error {
	membership, err := event.Membership()
	if err != nil {
		return fmt.Errorf("event.Membership: %w", err)
	}

	// "INSERT INTO syncapi_memberships (room_id, user_id, membership, event_id, stream_pos, topological_pos)" +
	// " VALUES ($1, $2, $3, $4, $5, $6)" +
	// " ON CONFLICT (room_id, user_id, membership)" +
	// " DO UPDATE SET event_id = $4, stream_pos = $5, topological_pos = $6"

	// _, err = sqlutil.TxStmt(txn, s.upsertMembershipStmt).ExecContext(
	// 	ctx,
	// 	event.RoomID(),
	// 	*event.StateKey(),
	// 	membership,
	// 	event.EventID(),
	// 	streamPos,
	// 	topologicalPos,
	// )

	// 	UNIQUE (room_id, user_id, membership)
	docId := fmt.Sprintf("%s,%s,%s", event.RoomID(), *event.StateKey(), membership)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getMembership(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		// " DO UPDATE SET event_id = $4, stream_pos = $5, topological_pos = $6"
		dbData.SetUpdateTime()
		dbData.Membership.EventID = event.EventID()
		dbData.Membership.StreamPos = int64(streamPos)
		dbData.Membership.TopologicalPos = int64(topologicalPos)
	} else {
		data := membershipCosmos{
			RoomID:         event.RoomID(),
			UserID:         *event.StateKey(),
			Membership:     membership,
			EventID:        event.EventID(),
			StreamPos:      int64(streamPos),
			TopologicalPos: int64(topologicalPos),
		}

		dbData = &membershipCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			Membership:     data,
		}
	}

	return cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		dbData)
}

func (s *membershipsStatements) SelectMembership(
	ctx context.Context, txn *sql.Tx, roomID, userID, memberships []string,
) (eventID string, streamPos, topologyPos types.StreamPosition, err error) {
	// params := []interface{}{roomID, userID}
	// for _, membership := range memberships {
	// 	params = append(params, membership)
	// }

	// "SELECT event_id, stream_pos, topological_pos FROM syncapi_memberships" +
	// " WHERE room_id = $1 AND user_id = $2 AND membership IN ($3)" +
	// " ORDER BY stream_pos DESC" +
	// " LIMIT 1"

	// err = sqlutil.TxStmt(txn, stmt).QueryRowContext(ctx, params...).Scan(&eventID, &streamPos, &topologyPos)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": userID,
		"@x4": memberships,
	}
	// orig := strings.Replace(selectMembershipSQL, "@x4", cosmosdbutil.QueryVariadicOffset(len(memberships), 2), 1)
	var rows []membershipCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), selectMembershipSQL, params, &rows)

	if err != nil || len(rows) == 0 {
		return "", 0, 0, err
	}
	// err = sqlutil.TxStmt(txn, stmt).QueryRowContext(ctx, params...).Scan(&eventID, &streamPos, &topologyPos)
	eventID = rows[0].Membership.EventID
	streamPos = types.StreamPosition(rows[0].Membership.StreamPos)
	topologyPos = types.StreamPosition(rows[0].Membership.TopologicalPos)
	return
}
