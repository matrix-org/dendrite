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
	"time"

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

type OutputRoomEventTopologyCosmos struct {
	EventID             string `json:"event_id"`
	TopologicalPosition int64  `json:"topological_position"`
	StreamPosition      int64  `json:"stream_position"`
	RoomID              string `json:"room_id"`
}

type OutputRoomEventTopologyCosmosData struct {
	Id                      string                        `json:"id"`
	Pk                      string                        `json:"_pk"`
	Tn                      string                        `json:"_sid"`
	Cn                      string                        `json:"_cn"`
	ETag                    string                        `json:"_etag"`
	Timestamp               int64                         `json:"_ts"`
	OutputRoomEventTopology OutputRoomEventTopologyCosmos `json:"mx_syncapi_output_room_event_topology"`
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

func queryOutputRoomEventTopology(s *outputRoomEventsTopologyStatements, ctx context.Context, qry string, params map[string]interface{}) ([]OutputRoomEventTopologyCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []OutputRoomEventTopologyCosmosData

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

func deleteOutputRoomEventTopology(s *outputRoomEventsTopologyStatements, ctx context.Context, dbData OutputRoomEventTopologyCosmosData) error {
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE(topological_position, room_id, stream_position)
	docId := fmt.Sprintf("%d_%s_%d", event.Depth(), event.RoomID(), pos)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := OutputRoomEventTopologyCosmos{
		EventID:             event.EventID(),
		TopologicalPosition: event.Depth(),
		RoomID:              event.RoomID(),
		StreamPosition:      int64(pos),
	}

	dbData := &OutputRoomEventTopologyCosmosData{
		Id:                      cosmosDocId,
		Tn:                      s.db.cosmosConfig.TenantName,
		Cn:                      dbCollectionName,
		Pk:                      pk,
		Timestamp:               time.Now().Unix(),
		OutputRoomEventTopology: data,
	}

	// _, err := sqlutil.TxStmt(txn, s.insertEventInTopologyStmt).ExecContext(
	// 	ctx, event.EventID(), event.Depth(), event.RoomID(), pos,
	// )

	var options = cosmosdbapi.GetUpsertDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return types.StreamPosition(event.Depth()), err
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": minDepth,
		"@x4": maxDepth,
		"@x5": maxDepth,
		"@x6": maxStreamPos,
		"@x7": limit,
	}

	rows, err := queryOutputRoomEventTopology(s, ctx, stmt, params)
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

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventID,
	}

	rows, err := queryOutputRoomEventTopology(s, ctx, s.selectPositionInTopologyStmt, params)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	rows, err := queryOutputRoomEventTopology(s, ctx, s.selectMaxPositionInTopologyStmt, params)
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
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	rows, err := queryOutputRoomEventTopology(s, ctx, s.deleteTopologyForRoomStmt, params)
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
