// Copyright 2018 Vector Creations Ltd
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

package storage

import (
	"context"
	"database/sql"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/encryptoapi/types"
)

const algorithmSchema = `
-- The media_repository table holds metadata for each media file stored and accessible to the local server,
-- the actual file is stored separately.
CREATE TABLE IF NOT EXISTS encrypt_algorithm (
    device_id TEXT 				NOT NULL,
    user_id TEXT 				NOT NULL,
    algorithms TEXT 			NOT NULL
);
`
const insertalSQL = `
INSERT INTO encrypt_algorithm (device_id, user_id, algorithms) 
VALUES ($1, $2, $3)
`
const selectalSQL = `
SELECT user_id, device_id, algorithms FROM encrypt_algorithm 
WHERE user_id = $1 AND device_id = $2
`

type alStatements struct {
	insertAlStmt *sql.Stmt
	selectAlStmt *sql.Stmt
}

func (s *alStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(algorithmSchema)
	if err != nil {
		return
	}
	if s.insertAlStmt, err = db.Prepare(insertalSQL); err != nil {
		return
	}
	if s.selectAlStmt, err = db.Prepare(selectalSQL); err != nil {
		return
	}
	return
}

// persist algorithms
func (s *alStatements) insertAl(
	ctx context.Context, txn *sql.Tx,
	userID, deviceID, algorithms string,
) error {
	stmt := common.TxStmt(txn, s.insertAlStmt)
	_, err := stmt.ExecContext(ctx, deviceID, userID, algorithms)
	return err
}

// select algorithms
func (s *alStatements) selectAl(
	ctx context.Context,
	txn *sql.Tx,
	userID, deviceID string,
) (holder types.AlHolder, err error) {

	stmt := common.TxStmt(txn, s.selectAlStmt)
	row := stmt.QueryRowContext(ctx, userID, deviceID)
	single := types.AlHolder{}
	err = row.Scan(
		&single.UserID,
		&single.DeviceID,
		&single.SupportedAlgorithm,
	)
	return single, err
}
