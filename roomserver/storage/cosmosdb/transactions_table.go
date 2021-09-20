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
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
)

// const transactionsSchema = `
// 	CREATE TABLE IF NOT EXISTS roomserver_transactions (
// 		transaction_id TEXT NOT NULL,
// 		session_id INTEGER NOT NULL,
// 		user_id TEXT NOT NULL,
// 		event_id TEXT NOT NULL,
// 		PRIMARY KEY (transaction_id, session_id, user_id)
// 	);
// `

type TransactionCosmos struct {
	TransactionID string `json:"transaction_id"`
	SessionID     int64  `json:"session_id"`
	UserID        string `json:"user_id"`
	EventID       string `json:"event_id"`
}

type TransactionCosmosData struct {
	cosmosdbapi.CosmosDocument
	Transaction TransactionCosmos `json:"mx_roomserver_transaction"`
}

// const insertTransactionSQL = `
// 	INSERT INTO roomserver_transactions (transaction_id, session_id, user_id, event_id)
// 	  VALUES ($1, $2, $3, $4)
// `

// const selectTransactionEventIDSQL = `
// 	SELECT event_id FROM roomserver_transactions
// 	  WHERE transaction_id = $1 AND session_id = $2 AND user_id = $3
// `

type transactionStatements struct {
	db *Database
	// insertTransactionStmt        *sql.Stmt
	selectTransactionEventIDStmt *sql.Stmt
	tableName                    string
}

func getTransaction(s *transactionStatements, ctx context.Context, pk string, docId string) (*TransactionCosmosData, error) {
	response := TransactionCosmosData{}
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

func NewCosmosDBTransactionsTable(db *Database) (tables.Transactions, error) {
	s := &transactionStatements{
		db: db,
	}
	// return s, shared.StatementList{
	// 	{&s.insertTransactionStmt, insertTransactionSQL},
	// 	{&s.selectTransactionEventIDStmt, selectTransactionEventIDSQL},
	// }.Prepare(db)
	s.tableName = "transactions"
	return s, nil
}

func (s *transactionStatements) InsertTransaction(
	ctx context.Context, txn *sql.Tx,
	transactionID string,
	sessionID int64,
	userID string,
	eventID string,
) error {

	// INSERT INTO roomserver_transactions (transaction_id, session_id, user_id, event_id)
	// VALUES ($1, $2, $3, $4)
	data := TransactionCosmos{
		EventID:       eventID,
		SessionID:     sessionID,
		TransactionID: transactionID,
		UserID:        userID,
	}

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)

	// 		PRIMARY KEY (transaction_id, session_id, user_id)
	docId := fmt.Sprintf("%s_%d_%s", transactionID, sessionID, userID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	var dbData = TransactionCosmosData{
		CosmosDocument: cosmosdbapi.GenerateDocument(dbCollectionName, s.db.cosmosConfig.TenantName, pk, cosmosDocId),
		Transaction:    data,
	}

	var options = cosmosdbapi.GetCreateDocumentOptions(dbData.Pk)
	_, _, err := cosmosdbapi.GetClient(s.db.connection).CreateDocument(
		ctx,
		s.db.cosmosConfig.DatabaseName,
		s.db.cosmosConfig.ContainerName,
		&dbData,
		options)

	return err
}

func (s *transactionStatements) SelectTransactionEventID(
	ctx context.Context,
	transactionID string,
	sessionID int64,
	userID string,
) (eventID string, err error) {

	// SELECT event_id FROM roomserver_transactions
	// WHERE transaction_id = $1 AND session_id = $2 AND user_id = $3

	var dbCollectionName = cosmosdbapi.GetCollectionName(s.db.databaseName, s.tableName)
	// 		PRIMARY KEY (transaction_id, session_id, user_id)
	docId := fmt.Sprintf("%s_%d_%s", transactionID, sessionID, userID)
	cosmosDocId := cosmosdbapi.GetDocumentId(s.db.cosmosConfig.TenantName, dbCollectionName, docId)
	pk := cosmosdbapi.GetPartitionKey(s.db.cosmosConfig.TenantName, dbCollectionName)

	response, err := getTransaction(s, ctx, pk, cosmosDocId)

	if err != nil {
		return "", err
	}

	return response.Transaction.EventID, err
}
