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

// const outboundPeeksSchema = `
// CREATE TABLE IF NOT EXISTS federationsender_outbound_peeks (
// 	room_id TEXT NOT NULL,
// 	server_name TEXT NOT NULL,
// 	peek_id TEXT NOT NULL,
//     creation_ts INTEGER NOT NULL,
//     renewed_ts INTEGER NOT NULL,
//     renewal_interval INTEGER NOT NULL,
// 	UNIQUE (room_id, server_name, peek_id)
// );
// `

type OutboundPeekCosmos struct {
	RoomID            string `json:"room_id"`
	ServerName        string `json:"server_name"`
	PeekID            string `json:"peek_id"`
	CreationTimestamp int64  `json:"creation_ts"`
	RenewedTimestamp  int64  `json:"renewed_ts"`
	RenewalInterval   int64  `json:"renewal_interval"`
}

type OutboundPeekCosmosData struct {
	cosmosdbapi.CosmosDocument
	OutboundPeek OutboundPeekCosmos `json:"mx_federationsender_outbound_peek"`
}

// const insertOutboundPeekSQL = "" +
// 	"INSERT INTO federationsender_outbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1"
const selectOutboundPeeksSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_outbound_peek.room_id = @x2"

// const renewOutboundPeekSQL = "" +
// 	"UPDATE federationsender_outbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

// "DELETE FROM federationsender_outbound_peeks WHERE room_id = $1 and server_name = $2"
const deleteOutboundPeekSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_outbound_peek.room_id = @x2" +
	"and c.mx_federationsender_outbound_peek.server_name = @x3"

// "DELETE FROM federationsender_outbound_peeks WHERE room_id = $1"
const deleteOutboundPeeksSQL = "" +
	"select * from c where c._cn = @x1 " +
	"and c.mx_federationsender_outbound_peek.room_id = @x2"

type outboundPeeksStatements struct {
	db *Database
	// insertOutboundPeekStmt  *sql.Stmt
	// selectOutboundPeekStmt  *sql.Stmt
	selectOutboundPeeksStmt string
	// renewOutboundPeekStmt   *sql.Stmt
	deleteOutboundPeekStmt  string
	deleteOutboundPeeksStmt string
	tableName               string
}

func queryOutboundPeek(s *outboundPeeksStatements, ctx context.Context, qry string, params map[string]interface{}) ([]OutboundPeekCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []OutboundPeekCosmosData

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

func getOutboundPeek(s *outboundPeeksStatements, ctx context.Context, pk string, docId string) (*OutboundPeekCosmosData, error) {
	response := OutboundPeekCosmosData{}
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

func setOutboundPeek(s *outboundPeeksStatements, ctx context.Context, outboundPeek OutboundPeekCosmosData) (*OutboundPeekCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(outboundPeek.Pk, outboundPeek.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		outboundPeek.Id,
		&outboundPeek,
		optionsReplace)
	return &outboundPeek, ex
}

func deleteOutboundPeek(s *outboundPeeksStatements, ctx context.Context, dbData OutboundPeekCosmosData) error {
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

func NewCosmosDBOutboundPeeksTable(db *Database) (s *outboundPeeksStatements, err error) {
	s = &outboundPeeksStatements{
		db: db,
	}
	s.selectOutboundPeeksStmt = selectOutboundPeeksSQL
	s.deleteOutboundPeeksStmt = deleteOutboundPeeksSQL
	s.deleteOutboundPeekStmt = deleteOutboundPeekSQL
	s.tableName = "outbound_peeks"
	return
}

func (s *outboundPeeksStatements) InsertOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	// "INSERT INTO federationsender_outbound_peeks (room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval) VALUES ($1, $2, $3, $4, $5, $6)"

	// stmt := sqlutil.TxStmt(txn, s.insertOutboundPeekStmt)
	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s_%s_%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	dbData, _ := getOutboundPeek(s, ctx, pk, cosmosDocId)
	if dbData != nil {
		dbData.SetUpdateTime()
		dbData.OutboundPeek.RenewalInterval = renewalInterval
		dbData.OutboundPeek.RenewedTimestamp = nowMilli

	} else {
		data := OutboundPeekCosmos{
			RoomID:            roomID,
			ServerName:        string(serverName),
			PeekID:            peekID,
			CreationTimestamp: nowMilli,
			RenewedTimestamp:  nowMilli,
			RenewalInterval:   renewalInterval,
		}

		dbData = &OutboundPeekCosmosData{
			CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
			OutboundPeek:   data,
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

func (s *outboundPeeksStatements) RenewOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64,
) (err error) {
	// "UPDATE federationsender_outbound_peeks SET renewed_ts=$1, renewal_interval=$2 WHERE room_id = $3 and server_name = $4 and peek_id = $5"

	nowMilli := time.Now().UnixNano() / int64(time.Millisecond)
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s_%s_%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// _, err = sqlutil.TxStmt(txn, s.renewOutboundPeekStmt).ExecContext(ctx, nowMilli, renewalInterval, roomID, serverName, peekID)
	res, err := getOutboundPeek(s, ctx, pk, cosmosDocId)

	if err != nil {
		return
	}

	if res == nil {
		return
	}

	res.OutboundPeek.RenewedTimestamp = nowMilli
	res.OutboundPeek.RenewalInterval = renewalInterval

	_, err = setOutboundPeek(s, ctx, *res)
	return
}

func (s *outboundPeeksStatements) SelectOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (*types.OutboundPeek, error) {

	// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1 and server_name = $2 and peek_id = $3"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 	UNIQUE (room_id, server_name, peek_id)
	docId := fmt.Sprintf("%s_%s_%s", roomID, serverName, peekID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	// row := sqlutil.TxStmt(txn, s.selectOutboundPeeksStmt).QueryRowContext(ctx, roomID)
	row, err := getOutboundPeek(s, ctx, pk, cosmosDocId)

	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, nil
	}
	outboundPeek := types.OutboundPeek{}
	outboundPeek.RoomID = row.OutboundPeek.RoomID
	outboundPeek.ServerName = gomatrixserverlib.ServerName(row.OutboundPeek.ServerName)
	outboundPeek.PeekID = row.OutboundPeek.PeekID
	outboundPeek.CreationTimestamp = row.OutboundPeek.CreationTimestamp
	outboundPeek.RenewedTimestamp = row.OutboundPeek.RenewedTimestamp
	outboundPeek.RenewalInterval = row.OutboundPeek.RenewalInterval
	return &outboundPeek, nil
}

func (s *outboundPeeksStatements) SelectOutboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (outboundPeeks []types.OutboundPeek, err error) {

	// "SELECT room_id, server_name, peek_id, creation_ts, renewed_ts, renewal_interval FROM federationsender_outbound_peeks WHERE room_id = $1"

	if err != nil {
		return
	}
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	// rows, err := sqlutil.TxStmt(txn, s.selectOutboundPeeksStmt).QueryContext(ctx, roomID)
	rows, err := queryOutboundPeek(s, ctx, s.selectOutboundPeeksStmt, params)

	if err != nil {
		return
	}

	for _, item := range rows {
		outboundPeek := types.OutboundPeek{}
		outboundPeek.RoomID = item.OutboundPeek.RoomID
		outboundPeek.ServerName = gomatrixserverlib.ServerName(item.OutboundPeek.ServerName)
		outboundPeek.PeekID = item.OutboundPeek.PeekID
		outboundPeek.CreationTimestamp = item.OutboundPeek.CreationTimestamp
		outboundPeek.RenewedTimestamp = item.OutboundPeek.RenewedTimestamp
		outboundPeek.RenewalInterval = item.OutboundPeek.RenewalInterval
		outboundPeeks = append(outboundPeeks, outboundPeek)
	}

	return outboundPeeks, nil
}

func (s *outboundPeeksStatements) DeleteOutboundPeek(
	ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string,
) (err error) {

	// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1 and server_name = $2"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
		"@x3": serverName,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteOutboundPeekStmt).ExecContext(ctx, roomID, serverName, peekID)
	rows, err := queryOutboundPeek(s, ctx, s.deleteOutboundPeekStmt, params)

	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteOutboundPeek(s, ctx, item)
		if err != nil {
			return
		}
	}

	return
}

func (s *outboundPeeksStatements) DeleteOutboundPeeks(
	ctx context.Context, txn *sql.Tx, roomID string,
) (err error) {

	// "DELETE FROM federationsender_inbound_peeks WHERE room_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": roomID,
	}

	// _, err = sqlutil.TxStmt(txn, s.deleteOutboundPeeksStmt).ExecContext(ctx, roomID)
	rows, err := queryOutboundPeek(s, ctx, s.deleteOutboundPeeksStmt, params)

	if err != nil {
		return
	}

	for _, item := range rows {
		err = deleteOutboundPeek(s, ctx, item)
		if err != nil {
			return
		}
	}

	return
}
