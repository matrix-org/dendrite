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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

var oneTimeCryptoIDsSchema = `
-- Stores one-time cryptoIDs for users
CREATE TABLE IF NOT EXISTS keyserver_one_time_cryptoids (
    user_id TEXT NOT NULL,
	key_id TEXT NOT NULL,
	algorithm TEXT NOT NULL,
	ts_added_secs BIGINT NOT NULL,
	key_json TEXT NOT NULL,
	-- Clobber based on 3-uple of user/key/algorithm.
    CONSTRAINT keyserver_one_time_cryptoids_unique UNIQUE (user_id, key_id, algorithm)
);

CREATE INDEX IF NOT EXISTS keyserver_one_time_cryptoids_idx ON keyserver_one_time_cryptoids (user_id);
`

const upsertCryptoIDsSQL = "" +
	"INSERT INTO keyserver_one_time_cryptoids (user_id, key_id, algorithm, ts_added_secs, key_json)" +
	" VALUES ($1, $2, $3, $4, $5)" +
	" ON CONFLICT ON CONSTRAINT keyserver_one_time_cryptoids_unique" +
	" DO UPDATE SET key_json = $5"

const selectOneTimeCryptoIDsSQL = "" +
	"SELECT concat(algorithm, ':', key_id) as algorithmwithid, key_json FROM keyserver_one_time_cryptoids WHERE user_id=$1 AND concat(algorithm, ':', key_id) = ANY($2);"

const selectCryptoIDsCountSQL = "" +
	"SELECT algorithm, COUNT(key_id) FROM " +
	" (SELECT algorithm, key_id FROM keyserver_one_time_cryptoids WHERE user_id = $1 LIMIT 100)" +
	" x GROUP BY algorithm"

const deleteOneTimeCryptoIDSQL = "" +
	"DELETE FROM keyserver_one_time_cryptoids WHERE user_id = $1 AND algorithm = $2 AND key_id = $3"

const selectCryptoIDByAlgorithmSQL = "" +
	"SELECT key_id, key_json FROM keyserver_one_time_cryptoids WHERE user_id = $1 AND algorithm = $2 LIMIT 1"

const deleteOneTimeCryptoIDsSQL = "" +
	"DELETE FROM keyserver_one_time_cryptoids WHERE user_id = $1"

type oneTimeCryptoIDsStatements struct {
	db                            *sql.DB
	upsertCryptoIDsStmt           *sql.Stmt
	selectCryptoIDsStmt           *sql.Stmt
	selectCryptoIDsCountStmt      *sql.Stmt
	selectCryptoIDByAlgorithmStmt *sql.Stmt
	deleteOneTimeCryptoIDStmt     *sql.Stmt
	deleteOneTimeCryptoIDsStmt    *sql.Stmt
}

func NewPostgresOneTimeCryptoIDsTable(db *sql.DB) (tables.OneTimeCryptoIDs, error) {
	s := &oneTimeCryptoIDsStatements{
		db: db,
	}
	_, err := db.Exec(oneTimeCryptoIDsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.upsertCryptoIDsStmt, upsertCryptoIDsSQL},
		{&s.selectCryptoIDsStmt, selectOneTimeCryptoIDsSQL},
		{&s.selectCryptoIDsCountStmt, selectCryptoIDsCountSQL},
		{&s.selectCryptoIDByAlgorithmStmt, selectCryptoIDByAlgorithmSQL},
		{&s.deleteOneTimeCryptoIDStmt, deleteOneTimeCryptoIDSQL},
		{&s.deleteOneTimeCryptoIDsStmt, deleteOneTimeCryptoIDsSQL},
	}.Prepare(db)
}

func (s *oneTimeCryptoIDsStatements) SelectOneTimeCryptoIDs(ctx context.Context, userID string, keyIDsWithAlgorithms []string) (map[string]json.RawMessage, error) {
	rows, err := s.selectCryptoIDsStmt.QueryContext(ctx, userID, pq.Array(keyIDsWithAlgorithms))
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCryptoIDsStmt: rows.close() failed")

	result := make(map[string]json.RawMessage)
	var (
		algorithmWithID string
		keyJSONStr      string
	)
	for rows.Next() {
		if err := rows.Scan(&algorithmWithID, &keyJSONStr); err != nil {
			return nil, err
		}
		result[algorithmWithID] = json.RawMessage(keyJSONStr)
	}
	return result, rows.Err()
}

func (s *oneTimeCryptoIDsStatements) CountOneTimeCryptoIDs(ctx context.Context, userID string) (*api.OneTimeCryptoIDsCount, error) {
	counts := &api.OneTimeCryptoIDsCount{
		UserID:   userID,
		KeyCount: make(map[string]int),
	}
	rows, err := s.selectCryptoIDsCountStmt.QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCryptoIDsCountStmt: rows.close() failed")
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

func (s *oneTimeCryptoIDsStatements) InsertOneTimeCryptoIDs(ctx context.Context, txn *sql.Tx, keys api.OneTimeCryptoIDs) (*api.OneTimeCryptoIDsCount, error) {
	now := time.Now().Unix()
	counts := &api.OneTimeCryptoIDsCount{
		UserID:   keys.UserID,
		KeyCount: make(map[string]int),
	}
	for keyIDWithAlgo, keyJSON := range keys.KeyJSON {
		algo, keyID := keys.Split(keyIDWithAlgo)
		_, err := sqlutil.TxStmt(txn, s.upsertCryptoIDsStmt).ExecContext(
			ctx, keys.UserID, keyID, algo, now, string(keyJSON),
		)
		if err != nil {
			return nil, err
		}
	}
	rows, err := sqlutil.TxStmt(txn, s.selectCryptoIDsCountStmt).QueryContext(ctx, keys.UserID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCryptoIDsCountStmt: rows.close() failed")
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

func (s *oneTimeCryptoIDsStatements) SelectAndDeleteOneTimeCryptoID(
	ctx context.Context, txn *sql.Tx, userID, algorithm string,
) (map[string]json.RawMessage, error) {
	var keyID string
	var keyJSON string
	err := sqlutil.TxStmtContext(ctx, txn, s.selectCryptoIDByAlgorithmStmt).QueryRowContext(ctx, userID, algorithm).Scan(&keyID, &keyJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	_, err = sqlutil.TxStmtContext(ctx, txn, s.deleteOneTimeCryptoIDStmt).ExecContext(ctx, userID, algorithm, keyID)
	return map[string]json.RawMessage{
		algorithm + ":" + keyID: json.RawMessage(keyJSON),
	}, err
}

func (s *oneTimeCryptoIDsStatements) DeleteOneTimeCryptoIDs(ctx context.Context, txn *sql.Tx, userID string) error {
	_, err := sqlutil.TxStmt(txn, s.deleteOneTimeCryptoIDsStmt).ExecContext(ctx, userID)
	return err
}
