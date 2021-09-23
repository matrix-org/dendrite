// Copyright 2018 New Vector Ltd
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

// const outputRoomEventsTopologySchema = `
// -- Stores output room events received from the roomserver.
// CREATE TABLE IF NOT EXISTS syncapi_output_room_events_topology (
//   event_id TEXT PRIMARY KEY,
//   topological_position BIGINT NOT NULL,
//   stream_position BIGINT NOT NULL,
//   room_id TEXT NOT NULL,

// 	UNIQUE(topological_position, room_id, stream_position)
// );
// -- The topological order will be used in events selection and ordering
// -- CREATE UNIQUE INDEX IF NOT EXISTS syncapi_event_topological_position_idx ON syncapi_output_room_events_topology(topological_position, stream_position, room_id);
// `

type outputRoomEventTopologyCosmos struct {
	EventID             string `json:"event_id"`
	TopologicalPosition int64  `json:"topological_position"`
	StreamPosition      int64  `json:"stream_position"`
	RoomID              string `json:"room_id"`
}

type outputRoomEventTopologyCosmosData struct {
	cosmosdbapi.CosmosDocument
	OutputRoomEventTopology outputRoomEventTopologyCosmos `json:"mx_syncapi_output_room_event_topology"`
}

// const insertEventInTopologySQL = "" +
// 	"INSERT INTO syncapi_output_room_events_topology (event_id, topological_position, room_id, stream_position)" +
// 	" VALUES ($1, $2, $3, $4)" +
// 	" ON CONFLICT DO NOTHING"

// "SELECT event_id FROM syncapi_output_room_events_topology" +
// " WHERE room_id = $1 AND (" +
// "(topological_position > $2 AND topological_position < $3) OR" +
// "(topological_position = $4 AND stream_position <= $5)" +
// ") ORDER BY topological_position ASC, stream_position ASC LIMIT $6"
const selectEventIDsInRangeASCSQL = "" +
	"select top @x7 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.room_id = @x2 " +
	"and ( " +
	"(c.mx_syncapi_output_room_event_topology.topological_position > @x3 and c.mx_syncapi_output_room_event_topology.topological_position < @x4) " +
	"OR " +
	"(c.mx_syncapi_output_room_event_topology.topological_position = @x5 and c.mx_syncapi_output_room_event_topology.stream_position < @x6) " +
	") " +
	"order by c.mx_syncapi_output_room_event_topology.topological_position asc "
	// ", c.mx_syncapi_output_room_event_topology.stream_position asc "

// "SELECT event_id  FROM syncapi_output_room_events_topology" +
// " WHERE room_id = $1 AND (" +
// "(topological_position > $2 AND topological_position < $3) OR" +
// "(topological_position = $4 AND stream_position <= $5)" +
// ") ORDER BY topological_position DESC, stream_position DESC LIMIT $6"
const selectEventIDsInRangeDESCSQL = "" +
	"select top @x7 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.room_id = @x2 " +
	"and ( " +
	"(c.mx_syncapi_output_room_event_topology.topological_position > @x3 and c.mx_syncapi_output_room_event_topology.topological_position < @x4) " +
	"OR " +
	"(c.mx_syncapi_output_room_event_topology.topological_position = @x5 and c.mx_syncapi_output_room_event_topology.stream_position < @x6) " +
	") " +
	"order by c.mx_syncapi_output_room_event_topology.topological_position desc "
	// ", c.mx_syncapi_output_room_event_topology.stream_position desc "

// "SELECT topological_position, stream_position FROM syncapi_output_room_events_topology" +
// " WHERE event_id = $1"
const selectPositionInTopologySQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.event_id = @x2 "

// "SELECT MAX(topological_position), stream_position FROM syncapi_output_room_events_topology" +
// " WHERE room_id = $1 ORDER BY stream_position DESC"

// "SELECT topological_position, stream_position FROM syncapi_output_room_events_topology" +
// " WHERE topological_position=(" +
// "SELECT MAX(topological_position) FROM syncapi_output_room_events_topology WHERE room_id=$1" +
// ") ORDER BY stream_position DESC LIMIT 1"
const selectMaxPositionInTopologySQL = "" +
	"select top 1 * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.topological_position = " +
	"( " +
	"select max(c.mx_syncapi_output_room_event_topology.topological_position) from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.room_id = @x2" +
	") " +
	"order by c.mx_syncapi_output_room_event_topology.stream_position desc "

// "DELETE FROM syncapi_output_room_events_topology WHERE room_id = $1"
const deleteTopologyForRoomSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_syncapi_output_room_event_topology.room_id = @x2 "

type outputRoomEventsTopologyStatements struct {
	db *SyncServerDatasource
	// insertEventInTopologyStmt       *sql.Stmt
	selectEventIDsInRangeASCStmt    string
	selectEventIDsInRangeDESCStmt   string
	selectPositionInTopologyStmt    string
	selectMaxPositionInTopologyStmt string
	deleteTopologyForRoomStmt       string
	tableName                       string
}

func (s *outputRoomEventsTopologyStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *outputRoomEventsTopologyStatements) getPartitionKey() string {
	//No easy PK, so just use the collection
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getOutputRoomEventTopology(s *outputRoomEventsTopologyStatements, ctx context.Context, pk string, docId string) (*outputRoomEventTopologyCosmosData, error) {
	response := outputRoomEventTopologyCosmosData{}
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

func deleteOutputRoomEventTopology(s *outputRoomEventsTopologyStatements, ctx context.Context, dbData outputRoomEventTopologyCosmosData) error {
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

func NewCosmosDBTopologyTable(db *SyncServerDatasource) (tables.Topology, error) {
	s := &outputRoomEventsTopologyStatements{
		db: db,
	}

	s.selectEventIDsInRangeASCStmt = selectEventIDsInRangeASCSQL
	s.selectEventIDsInRangeDESCStmt = selectEventIDsInRangeDESCSQL
	s.selectPositionInTopologyStmt = selectPositionInTopologySQL
	s.selectMaxPositionInTopologyStmt = selectMaxPositionInTopologySQL
	s.deleteTopologyForRoomStmt = deleteTopologyForRoomSQL
	s.tableName = "output_room_events_topology"
	return s, nil
}

// insertEventInTopology inserts the given event in the room's topology, based
// on the event's depth.
func (s *outputRoomEventsTopologyStatements) InsertEventInTopology(
	ctx context.Context, txn *sql.Tx, event *gomatrixserverlib.HeaderedEvent, pos types.StreamPosition,
) (types.StreamPosition, error) {

	// "INSERT INTO syncapi_output_room_events_topology (event_id, topological_position, room_id, stream_position)" +
	// 	" VALUES ($1, $2, $3, $4)" +
	// 	" ON CONFLICT DO NOTHING"

	// 	UNIQUE(topological_position, room_id, stream_position)
	docId := fmt.Sprintf("%d_%s_%d", event.Depth(), event.RoomID(), pos)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	var err error
	dbData, _ := getOutputRoomEventTopology(s, ctx, s.getPartitionKey(), cosmosDocId)
	if dbData != nil {
		// 	" ON CONFLICT DO NOTHING"
	} else {
		data := outputRoomEventTopologyCosmos{
			EventID:             event.EventID(),
			TopologicalPosition: event.Depth(),
			RoomID:              event.RoomID(),
			StreamPosition:      int64(pos),
		}

		dbData = &outputRoomEventTopologyCosmosData{
			CosmosDocument:          cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
			OutputRoomEventTopology: data,
		}
		// _, err := sqlutil.TxStmt(txn, s.insertEventInTopologyStmt).ExecContext(
		// 	ctx, event.EventID(), event.Depth(), event.RoomID(), pos,
		// )

		err = cosmosdbapi.UpsertDocument(ctx,
			s.db.connection,
			s.db.cosmosConfig.DatabaseName,
			s.db.cosmosConfig.ContainerName,
			dbData.Pk,
			dbData)
	}

	return types.StreamPosition(dbData.OutputRoomEventTopology.TopologicalPosition), err
}

func (s *outputRoomEventsTopologyStatements) SelectEventIDsInRange(
	ctx context.Context, txn *sql.Tx, roomID string,
	minDepth, maxDepth, maxStreamPos types.StreamPosition,
	limit int, chronologicalOrder bool,
) (eventIDs []string, err error) {
	// Decide on the selection's order according to whether chronological order
	// is requested or not.
	var stmt string
	if chronologicalOrder {
		// "SELECT event_id FROM syncapi_output_room_events_topology" +
		// " WHERE room_id = $1 AND (" +
		// "(topological_position > $2 AND topological_position < $3) OR" +
		// "(topological_position = $4 AND stream_position <= $5)" +
		// ") ORDER BY topological_position ASC, stream_position ASC LIMIT $6"
		stmt = s.selectEventIDsInRangeASCStmt
	} else {
		// "SELECT event_id  FROM syncapi_output_room_events_topology" +
		// " WHERE room_id = $1 AND (" +
		// "(topological_position > $2 AND topological_position < $3) OR" +
		// "(topological_position = $4 AND stream_position <= $5)" +
		// ") ORDER BY topological_position DESC, stream_position DESC LIMIT $6"
		stmt = s.selectEventIDsInRangeDESCStmt
	}

	// Query the event IDs.
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": minDepth,
		"@x4": maxDepth,
		"@x5": maxDepth,
		"@x6": maxStreamPos,
		"@x7": limit,
	}
	var rows []outputRoomEventTopologyCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), stmt, params, &rows)
	// rows, err := stmt.QueryContext(ctx, roomID, minDepth, maxDepth, maxDepth, maxStreamPos, limit)

	if err == sql.ErrNoRows {
		// If no event matched the request, return an empty slice.
		return []string{}, nil
	} else if err != nil {
		return
	}

	// Return the IDs.
	var eventID string
	for _, item := range rows {
		// if err = rows.Scan(&eventID); err != nil {
		// 	return
		// }
		eventID = item.OutputRoomEventTopology.EventID
		eventIDs = append(eventIDs, eventID)
	}

	return
}

// selectPositionInTopology returns the position of a given event in the
// topology of the room it belongs to.
func (s *outputRoomEventsTopologyStatements) SelectPositionInTopology(
	ctx context.Context, txn *sql.Tx, eventID string,
) (pos types.StreamPosition, spos types.StreamPosition, err error) {

	// "SELECT topological_position, stream_position FROM syncapi_output_room_events_topology" +
	// " WHERE event_id = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventID,
	}
	var rows []outputRoomEventTopologyCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectPositionInTopologyStmt, params, &rows)

	// stmt := sqlutil.TxStmt(txn, s.selectPositionInTopologyStmt)

	if err != nil {
		return
	}

	if len(rows) == 0 {
		return
	}

	// err = stmt.QueryRowContext(ctx, eventID).Scan(&pos, &spos)
	pos = types.StreamPosition(rows[0].OutputRoomEventTopology.TopologicalPosition)
	spos = types.StreamPosition(rows[0].OutputRoomEventTopology.StreamPosition)

	return
}

func (s *outputRoomEventsTopologyStatements) SelectMaxPositionInTopology(
	ctx context.Context, txn *sql.Tx, roomID string,
) (pos types.StreamPosition, spos types.StreamPosition, err error) {

	// "SELECT topological_position, stream_position FROM syncapi_output_room_events_topology" +
	// " WHERE topological_position=(" +
	// "SELECT MAX(topological_position) FROM syncapi_output_room_events_topology WHERE room_id=$1" +
	// ") ORDER BY stream_position DESC LIMIT 1"

	// stmt := sqlutil.TxStmt(txn, s.selectMaxPositionInTopologyStmt)
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
	}
	var rows []outputRoomEventTopologyCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectMaxPositionInTopologyStmt, params, &rows)

	// err = stmt.QueryRowContext(ctx, roomID).Scan(&pos, &spos)

	if err != nil {
		return
	}

	if len(rows) == 0 {
		return
	}

	pos = types.StreamPosition(rows[0].OutputRoomEventTopology.TopologicalPosition)
	spos = types.StreamPosition(rows[0].OutputRoomEventTopology.StreamPosition)
	return
}

func (s *outputRoomEventsTopologyStatements) DeleteTopologyForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {

	// "DELETE FROM syncapi_output_room_events_topology WHERE room_id = $1"
	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
	}
	var rows []outputRoomEventTopologyCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.deleteTopologyForRoomStmt, params, &rows)

	// _, err = sqlutil.TxStmt(txn, s.deleteTopologyForRoomStmt).ExecContext(ctx, roomID)

	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteOutputRoomEventTopology(s, ctx, item)
		if err != nil {
			return
		}
	}
	return err
}
