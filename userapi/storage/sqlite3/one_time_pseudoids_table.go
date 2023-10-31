// Copyright 2023 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
	"github.com/sirupsen/logrus"
)

var oneTimePseudoIDsSchema = `
-- Stores one-time pseudoIDs for users
CREATE TABLE IF NOT EXISTS keyserver_one_time_pseudoids (
    user_id TEXT NOT NULL,
	key_id TEXT NOT NULL,
	algorithm TEXT NOT NULL,
	ts_added_secs BIGINT NOT NULL,
	key_json TEXT NOT NULL,
	-- Clobber based on 3-uple of user/key/algorithm.
    UNIQUE (user_id, key_id, algorithm)
);

CREATE INDEX IF NOT EXISTS keyserver_one_time_pseudoids_idx ON keyserver_one_time_pseudoids (user_id);
`

const upsertPseudoIDsSQL = "" +
	"INSERT INTO keyserver_one_time_pseudoids (user_id, key_id, algorithm, ts_added_secs, key_json)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT (user_id, key_id, algorithm)" +
	" DO UPDATE SET key_json = $5"

const selectOneTimePseudoIDsSQL = "" +
	"SELECT key_id, algorithm, key_json FROM keyserver_one_time_pseudoids WHERE user_id=$1"

const selectPseudoIDsCountSQL = "" +
	"SELECT algorithm, COUNT(key_id) FROM " +
	" (SELECT algorithm, key_id FROM keyserver_one_time_pseudoids WHERE user_id = $1 LIMIT 100)" +
	" x GROUP BY algorithm"

const deleteOneTimePseudoIDSQL = "" +
	"DELETE FROM keyserver_one_time_pseudoids WHERE user_id = $1 AND algorithm = $2 AND key_id = $3"

const selectPseudoIDByAlgorithmSQL = "" +
	"SELECT key_id, key_json FROM keyserver_one_time_pseudoids WHERE user_id = $1 AND algorithm = $2 LIMIT 1"

const deleteOneTimePseudoIDsSQL = "" +
	"DELETE FROM keyserver_one_time_pseudoids WHERE user_id = $1"

type oneTimePseudoIDsStatements struct {
	db                            *sql.DB
	upsertPseudoIDsStmt           *sql.Stmt
	selectPseudoIDsStmt           *sql.Stmt
	selectPseudoIDsCountStmt      *sql.Stmt
	selectPseudoIDByAlgorithmStmt *sql.Stmt
	deleteOneTimePseudoIDStmt     *sql.Stmt
	deleteOneTimePseudoIDsStmt    *sql.Stmt
}

func NewSqliteOneTimePseudoIDsTable(db *sql.DB) (tables.OneTimePseudoIDs, error) {
	s := &oneTimePseudoIDsStatements{
		db: db,
	}
	_, err := db.Exec(oneTimePseudoIDsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.upsertPseudoIDsStmt, upsertPseudoIDsSQL},
		{&s.selectPseudoIDsStmt, selectOneTimePseudoIDsSQL},
		{&s.selectPseudoIDsCountStmt, selectPseudoIDsCountSQL},
		{&s.selectPseudoIDByAlgorithmStmt, selectPseudoIDByAlgorithmSQL},
		{&s.deleteOneTimePseudoIDStmt, deleteOneTimePseudoIDSQL},
		{&s.deleteOneTimePseudoIDsStmt, deleteOneTimePseudoIDsSQL},
	}.Prepare(db)
}

func (s *oneTimePseudoIDsStatements) SelectOneTimePseudoIDs(ctx context.Context, userID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	rows, err := s.selectPseudoIDsStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectPseudoIDsStmt: rows.close() failed")

	wantSet := make(map[string]bool, len(keyIDsWithAlgorithms))
	for _, ka := range keyIDsWithAlgorithms {
		wantSet[ka] = true
	}

	result := make(map[string]json.RawMessage)
	for rows.Next() {
		var keyID string
		var algorithm string
		var keyJSONStr string
		if err := rows.Scan(&keyID, &algorithm, &keyJSONStr); err != nil {
			return nil, err
		}
		keyIDWithAlgo := algorithm + ":" + keyID
		if wantSet[keyIDWithAlgo] {
			result[keyIDWithAlgo] = json.RawMessage(keyJSONStr)
		}
	}
	return result, rows.Err()
}

func (s *oneTimePseudoIDsStatements) CountOneTimePseudoIDs(ctx context.Context, userID string) (*api.OneTimePseudoIDsCount, error) {
	counts := &api.OneTimePseudoIDsCount{
		UserID:   userID,
		KeyCount: make(map[string]int),
	}
	rows, err := s.selectPseudoIDsCountStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectPseudoIDsCountStmt: rows.close() failed")
	for rows.Next() {
		var algorithm string
		var count int
		if err = rows.Scan(&algorithm, &count); err != nil {
			return nil, err
		}
		counts.KeyCount[algorithm] = count
	}
	return counts, nil
}

func (s *oneTimePseudoIDsStatements) InsertOneTimePseudoIDs(
	ctx context.Context, txn *sql.Tx, keys api.OneTimePseudoIDs,
) (*api.OneTimePseudoIDsCount, error) {
	now := time.Now().Unix()
	counts := &api.OneTimePseudoIDsCount{
		UserID:   keys.UserID,
		KeyCount: make(map[string]int),
	}
	for keyIDWithAlgo, keyJSON := range keys.KeyJSON {
		algo, keyID := keys.Split(keyIDWithAlgo)
		_, err := sqlutil.TxStmt(txn, s.upsertPseudoIDsStmt).ExecContext(
			ctx, keys.UserID, keyID, algo, now, string(keyJSON),
		)
		if err != nil {
			return nil, err
		}
	}
	rows, err := sqlutil.TxStmt(txn, s.selectPseudoIDsCountStmt).QueryContext(ctx, keys.UserID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectPseudoIDsCountStmt: rows.close() failed")
	for rows.Next() {
		var algorithm string
		var count int
		if err = rows.Scan(&algorithm, &count); err != nil {
			return nil, err
		}
		counts.KeyCount[algorithm] = count
	}

	return counts, rows.Err()
}

func (s *oneTimePseudoIDsStatements) SelectAndDeleteOneTimePseudoID(
	ctx context.Context, txn *sql.Tx, userID, algorithm string,
) (map[string]json.RawMessage, error) {
	var keyID string
	var keyJSON string
	err := sqlutil.TxStmtContext(ctx, txn, s.selectPseudoIDByAlgorithmStmt).QueryRowContext(ctx, userID, algorithm).Scan(&keyID, &keyJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			logrus.Warnf("No rows found for one time pseudoIDs")
			return nil, nil
		}
		return nil, err
	}
	_, err = sqlutil.TxStmtContext(ctx, txn, s.deleteOneTimePseudoIDStmt).ExecContext(ctx, userID, algorithm, keyID)
	if err != nil {
		return nil, err
	}
	if keyJSON == "" {
		logrus.Warnf("Empty key JSON for one time pseudoIDs")
		return nil, nil
	}
	return map[string]json.RawMessage{
		algorithm + ":" + keyID: json.RawMessage(keyJSON),
	}, err
}

func (s *oneTimePseudoIDsStatements) DeleteOneTimePseudoIDs(ctx context.Context, txn *sql.Tx, userID string) error {
	_, err := sqlutil.TxStmt(txn, s.deleteOneTimePseudoIDsStmt).ExecContext(ctx, userID)
	return err
}
