// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
)

// const inboundPeeksSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_inbound_peeks (
// 	room_id TEXT NOT NULL,
// 	server_name TEXT NOT NULL,
// 	peek_id TEXT NOT NULL,
//     creation_ts INTEGER NOT NULL,
//     renewed_ts INTEGER NOT NULL,
//     renewal_interval INTEGER NOT NULL,
// 	UNIQUE (room_id, server_name, peek_id)
// );
// `

type inboundPeekCosmos struct {
	RoomID            string `json:"room_id"`
	ServerName        string `json:"server_name"`
	PeekID            string `json:"peek_id"`
	CreationTimestamp int64  `json:"creation_ts"`
	RenewedTimestamp  int64  `json:"renewed_ts"`
	RenewalInterval   int64  `json:"renewal_interval"`
}

type inboundPeekCosmosData struct {
	cosmosdbapi.CosmosDocument
	InboundPeek inboundPeekCosmos `json:"mx_federationsender_inbound_peek"`
}

// const insertInboundPeekSQL = "" +
// 	"INSERT INTO federationsender_inbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

// const selectInboundPeekSQL = "" +
// 	"SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1"
const selectInboundPeeksSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_inbound_peek.room_id = @x2 "

// const renewInboundPeekSQL = "" +
// 	"UPDATE federationsender_inbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2"
const deleteInboundPeekSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_inbound_peek.room_id = @x2 " +
	"and c.mx_federationsender_inbound_peek.server_name = @x3 "

// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1"
const deleteInboundPeeksSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_inbound_peek.room_id = @x2 "

type inboundPeeksStatements struct {
	db *Database
	// insertInboundPeekStmt  *sql.Stmt
	// selectInboundPeekStmt  *sql.Stmt
	selectInboundPeeksStmt string
	// renewInboundPeekStmt   string
	deleteInboundPeekStmt  string
	deleteInboundPeeksStmt string
	tableName              string
}

func (s *inboundPeeksStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *inboundPeeksStatements) getPartitionKey(roomId string) string {
	uniqueId := roomId
	return cosmosdbapi.GetPartitionKeyByUniqueId(s.db.cosmosConfig.TenantName, s.getCollectionName(), uniqueId)
}

func getInboundPeek(s *inboundPeeksStatements, ctx context.Context, pk string, docId string) (*inboundPeekCosmosData, error) {
	response := inboundPeekCosmosData{}
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

func deleteInboundPeek(s *inboundPeeksStatements, ctx context.Context, dbData inboundPeekCosmosData) error {
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

func NewCosmosDBInboundPeeksTable(db *Database) (s *inboundPeeksStatements, err error) {
	s = &inboundPeeksStatements{
		db: db,
	}
	s.selectInboundPeeksStmt = selectInboundPeeksSQL
	s.deleteInboundPeeksStmt = deleteInboundPeeksSQL
	s.deleteInboundPeekStmt = deleteInboundPeekSQL
	s.tableName = "inbound_peeks"
	return
}

func (s *inboundPeeksStatements) InsertInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {

	// "INSERT INTO federationsender_inbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	// stmt := sqlutil.TxStmt(txn, s.insertInboundPeekStmt)

	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s,%s,%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	dbData, _ := getInboundPeek(s, ctx, s.getPartitionKey(roomID), cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.InboundPeek.RenewedTimestamp = nowMilli
		dbData.InboundPeek.RenewalInterval = renewalInterval
	} else {
		data := inboundPeekCosmos{
			RoomID:            roomID,
			ServerName:        string(serverName),
			PeekID:            peekID,
			CreationTimestamp: nowMilli,
			RenewedTimestamp:  nowMilli,
			RenewalInterval:   renewalInterval,
		}

		dbData = &inboundPeekCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(roomID), cosmosDocId),
			InboundPeek:    data,
		}
	}

	// _, err = stmt.ExecContext(ctx, roomID, serverName, peekID, nowMilli, nowMilli, renewalInterval)

	err = cosmosdbapi.UpsertDocument(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		dbData.Pk,
		&dbData)

	return
}

func (s *inboundPeeksStatements) RenewInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	// "UPDATE federationsender_inbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

	// _, err = sqlutil.TxStmt(txn, s.renewInboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s,%s,%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	// _, err = sqlutil.TxStmt(txn, s.renewInboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	item, err := getInboundPeek(s, ctx, s.getPartitionKey(roomID), cosmosDocId)

	if err != nil {
		return
	}

	if item == nil {
		return
	}

	item.SetUpdateTime()
	item.InboundPeek.RenewedTimestamp = nowMilli
	item.InboundPeek.RenewalInterval = renewalInterval

	_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)

	return
}

func (s *inboundPeeksStatements) SelectInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (*types.InboundPeek, error) {

	// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"
	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s,%s,%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getPartitionKey(roomID), docId)

	// row := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryRowContext(ctx, roomID)
	row, err := getInboundPeek(s, ctx, s.getPartitionKey(roomID), cosmosDocId)

	if row == nil {
		return nil, nil
	}
	inboundPeek := types.InboundPeek{}
	inboundPeek.RoomID = row.InboundPeek.RoomID
	inboundPeek.ServerName = gomatrixserverlib.ServerName(row.InboundPeek.ServerName)
	inboundPeek.PeekID = row.InboundPeek.PeekID
	inboundPeek.CreationTimestamp = row.InboundPeek.CreationTimestamp
	inboundPeek.RenewedTimestamp = row.InboundPeek.RenewedTimestamp
	inboundPeek.RenewalInterval = row.InboundPeek.RenewalInterval
	if err != nil {
		return nil, err
	}
	return &inboundPeek, nil
}

func (s *inboundPeeksStatements) SelectInboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (inboundPeeks []types.InboundPeek, err error) {
	// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_inbound_peeks WHERE room_id = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
	}

	// rows, err := sqlutil.TxStmt(txn, s.selectInboundPeeksStmt).QueryContext(ctx, roomID)
	var rows []inboundPeekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(roomID), s.selectInboundPeeksStmt, params, &rows)

	if err != nil {
		return
	}

	for _, item := range rows {
		inboundPeek := types.InboundPeek{}
		inboundPeek.RoomID = item.InboundPeek.RoomID
		inboundPeek.ServerName = gomatrixserverlib.ServerName(item.InboundPeek.ServerName)
		inboundPeek.PeekID = item.InboundPeek.PeekID
		inboundPeek.CreationTimestamp = item.InboundPeek.CreationTimestamp
		inboundPeek.RenewedTimestamp = item.InboundPeek.RenewedTimestamp
		inboundPeek.RenewalInterval = item.InboundPeek.RenewalInterval
		inboundPeeks = append(inboundPeeks, inboundPeek)
	}

	return inboundPeeks, nil
}

func (s *inboundPeeksStatements) DeleteInboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (err error) {
	// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
		"@x3": serverName,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteInboundPeekStmt).ExecContext(ctx, roomID, serverName, peekID)
	var rows []inboundPeekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(roomID), s.deleteInboundPeekStmt, params, &rows)

	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteInboundPeek(s, ctx, item)
		if err != nil {
			return
		}
	}

	return
}

func (s *inboundPeeksStatements) DeleteInboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {
	// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1"

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": roomID,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteInboundPeeksStmt).ExecContext(ctx, roomID)
	var rows []inboundPeekCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(roomID), s.deleteInboundPeekStmt, params, &rows)

	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteInboundPeek(s, ctx, item)
		if err != nil {
			return
		}
	}
	return
}
