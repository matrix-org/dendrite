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

type redactionCosmos struct {
	RedactionEventID string `json:"redaction_event_id"`
	RedactsEventID   string `json:"redacts_event_id"`
	Validated        bool   `json:"validated"`
}

type redactionCosmosData struct {
	cosmosdbapi.CosmosDocument
	Redaction redactionCosmos `json:"mx_roomserver_redaction"`
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

func (s *redactionStatements) getCollectionName() string {
	return cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
}

func (s *redactionStatements) getPartitionKey() string {
	return cosmosdbapi.GetPartitionKeyByCollection(s.db.cosmosConfig.TenantName, s.getCollectionName())
}

func getRedaction(s *redactionStatements, ctx context.Context, pk string, docId string) (*redactionCosmosData, error) {
	response := redactionCosmosData{}
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

	//     redaction_event_id TEXT PRIMARY KEY,
	docId := info.RedactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	data := redactionCosmos{
		RedactionEventID: info.RedactionEventID,
		RedactsEventID:   info.RedactsEventID,
		Validated:        info.Validated,
	}

	var dbData = redactionCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(s.getCollectionName(), s.db.cosmosConfig.TenantName, s.getPartitionKey(), cosmosDocId),
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
	//     redaction_event_id TEXT PRIMARY KEY,
	docId := redactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	response, err := getRedaction(s, ctx, s.getPartitionKey(), cosmosDocId)
	if err != nil {
		info = nil
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

	params := map[string]interface{}{
		"@x1": s.getCollectionName(),
		"@x2": eventID,
	}
	var rows []redactionCosmosData
	err = cosmosdbapi.PerformQuery(ctx,
		s.db.connection,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		s.getPartitionKey(), s.selectRedactionInfoByEventBeingRedactedStmt, params, &rows)

	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		info = nil
		err = nil
		return
	}
	// TODO: Check this is ok to return the 1st one
	*info = tables.RedactionInfo{
		RedactionEventID: rows[0].Redaction.RedactionEventID,
		RedactsEventID:   rows[0].Redaction.RedactsEventID,
		Validated:        rows[0].Redaction.Validated,
	}
	return
}

func (s *redactionStatements) MarkRedactionValidated(
	ctx context.Context, txn *sql.Tx, redactionEventID string, validated bool,
) error {

	// " UPDATE roomserver_redactions SET validated = $2 WHERE redaction_event_id = $1"

	//     redaction_event_id TEXT PRIMARY KEY,
	docId := redactionEventID
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, s.getCollectionName(), docId)

	item, err := getRedaction(s, ctx, s.getPartitionKey(), cosmosDocId)
	if err != nil {
		return err
	}

	item.SetUpdateTime()
	item.Redaction.Validated = validated

	_, err = cosmosdbapi.UpdateDocument(ctx, s.db.connection, s.db.cosmosConfig.DatabaseName, s.db.cosmosConfig.ContainerName, item.Pk, item.ETag, item.Id, item)
	return err
}
