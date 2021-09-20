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

	"github.com/matrix-org/dendrite/internal/cosmosdbapi"
	"github.com/matrix-org/dendrite/internal/cosmosdbutil"

	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

// const redactionsSchema = `
// -- Stores information about the redacted state of events.
// -- We need to track redactions rather than blindly updating the event JSON table on receipt of a redaction
// -- because we might receive the redaction BEFORE we receive the event which it redacts (think backfill).
// CREATE TABLE IF NOT EXISTS roomserver_redactions (
//     redaction_event_id TEXT PRIMARY KEY,
// 	redacts_event_id TEXT NOT NULL,
// 	-- Initially FALSE, set to TRUE when the redaction has been validated according to rooms v3+ spec
// 	-- https://matrix.org/docs/spec/rooms/v3#authorization-rules-for-events
// 	validated BOOLEAN NOT NULL
// );
// `

type RedactionCosmos struct {
	RedactionEventID string `json:"redaction_event_id"`
	RedactsEventID   string `json:"redacts_event_id"`
	Validated        bool   `json:"validated"`
}

type RedactionCosmosData struct {
	cosmosdbapi.CosmosDocument
	Redaction RedactionCosmos `json:"mx_roomserver_redaction"`
}

// const insertRedactionSQL = "" +
// 	"INSERT OR IGNORE INTO roomserver_redactions (redaction_event_id, redacts_event_id, validated)" +
// 	" VALUES ($1, $2, $3)"

// const selectRedactionInfoByRedactionEventIDSQL = "" +
// 	"SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
// 	" WHERE redaction_event_id = $1"

// "SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
// " WHERE redacts_event_id = $1"
const selectRedactionInfoByEventBeingRedactedSQL = "" +
	"select * from c where c._cn = @x1 " +
	" and c.mx_roomserver_redaction.redacts_event_id = @x2"

// const markRedactionValidatedSQL = "" +
// 	" UPDATE roomserver_redactions SET validated = $2 WHERE redaction_event_id = $1"

type redactionStatements struct {
	db *Database
	// insertRedactionStmt                         *sql.Stmt
	// selectRedactionInfoByRedactionEventIDStmt   *sql.Stmt
	selectRedactionInfoByEventBeingRedactedStmt string
	// markRedactionValidatedStmt                  *sql.Stmt
	tableName string
}

func queryRedaction(s *redactionStatements, ctx context.Context, qry string, params map[string]interface{}) ([]RedactionCosmosData, error) {
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	var pk = cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)
	var response []RedactionCosmosData

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

func getRedaction(s *redactionStatements, ctx context.Context, pk string, docId string) (*RedactionCosmosData, error) {
	response := RedactionCosmosData{}
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

func setRedaction(s *redactionStatements, ctx context.Context, redaction RedactionCosmosData) (*RedactionCosmosData, error) {
	var optionsReplace = cosmosdbapi.GetReplaceDocumentOptions(redaction.Pk, redaction.ETag)
	var _, _, ex = cosmosdbapi.GetClient(s.db.connection).ReplaceDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		redaction.Id,
		&redaction,
		optionsReplace)
	return &redaction, ex
}

func NewCosmosDBRedactionsTable(db *Database) (tables.Redactions, error) {
	s := &redactionStatements{
		db: db,
	}

	// return s, shared.StatementList{
	// 	{&s.insertRedactionStmt, insertRedactionSQL},
	// 	{&s.selectRedactionInfoByRedactionEventIDStmt, selectRedactionInfoByRedactionEventIDSQL},
	s.selectRedactionInfoByEventBeingRedactedStmt = selectRedactionInfoByEventBeingRedactedSQL
	// 	{&s.markRedactionValidatedStmt, markRedactionValidatedSQL},
	// }.Prepare(db)
	s.tableName = "redactions"
	return s, nil
}

func (s *redactionStatements) InsertRedaction(
	ctx context.Context, txn *sql.Tx, info tables.RedactionInfo,
) error {

	// "INSERT OR IGNORE INTO roomserver_redactions (redaction_event_id, redacts_event_id, validated)" +
	// " VALUES ($1, $2, $3)"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     redaction_event_id TEXT PRIMARY KEY,
	docId := info.RedactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	data := RedactionCosmos{
		RedactionEventID: info.RedactionEventID,
		RedactsEventID:   info.RedactsEventID,
		Validated:        info.Validated,
	}

	var dbData = RedactionCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
		Redaction:      data,
	}

	// "INSERT OR IGNORE INTO roomserver_redactions (redaction_event_id, redacts_event_id, validated)" +
	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	// TODO: Just forDebug - Remove exception
	if err != nil {
		return err
	}

	//Ignore Error
	return nil
}

func (s *redactionStatements) SelectRedactionInfoByRedactionEventID(
	ctx context.Context, txn *sql.Tx, redactionEventID string,
) (info *tables.RedactionInfo, err error) {
	info = &tables.RedactionInfo{}

	// "SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
	// " WHERE redaction_event_id = $1"
	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     redaction_event_id TEXT PRIMARY KEY,
	docId := redactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	response, err := getRedaction(s, ctx, pk, cosmosDocId)
	if err != nil {
		info = nil
		err = err
		return
	}

	if response == nil {
		info = nil
		err = nil
		return
	}
	info = &tables.RedactionInfo{
		RedactionEventID: response.Redaction.RedactionEventID,
		RedactsEventID:   response.Redaction.RedactsEventID,
		Validated:        response.Redaction.Validated,
	}
	return
}

func (s *redactionStatements) SelectRedactionInfoByEventBeingRedacted(
	ctx context.Context, txn *sql.Tx, eventID string,
) (info *tables.RedactionInfo, err error) {

	// "SELECT redaction_event_id, redacts_event_id, validated FROM roomserver_redactions" +
	// " WHERE redacts_event_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	params := map[string]interface{}{
		"@x1": dbCollectionName,
		"@x2": eventID,
	}
	response, err := queryRedaction(s, ctx, s.selectRedactionInfoByEventBeingRedactedStmt, params)

	if err != nil {
		return nil, err
	}

	if len(response) == 0 {
		info = nil
		err = nil
		return
	}
	// TODO: Check this is ok to return the 1st one
	*info = tables.RedactionInfo{
		RedactionEventID: response[0].Redaction.RedactionEventID,
		RedactsEventID:   response[0].Redaction.RedactsEventID,
		Validated:        response[0].Redaction.Validated,
	}
	return
}

func (s *redactionStatements) MarkRedactionValidated(
	ctx context.Context, txn *sql.Tx, redactionEventID string, validated bool,
) error {

	// " UPDATE roomserver_redactions SET validated = $2 WHERE redaction_event_id = $1"

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	//     redaction_event_id TEXT PRIMARY KEY,
	docId := redactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	response, err := getRedaction(s, ctx, pk, cosmosDocId)
	if err != nil {
		return err
	}

	response.Redaction.Validated = validated

	_, err = setRedaction(s, ctx, *response)
	return err
}
