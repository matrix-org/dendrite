// Copyright 2021 The Matrix.org Foundation C.I.C.
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
	"fmt"

	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/keyserver/storage/tables"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/gomatrixserverlib"
)

var crossSigningSigsSchema = `
CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs (
    origin_user_id TEXT NOT NULL,
	origin_key_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	target_key_id TEXT NOT NULL,
	signature TEXT NOT NULL,
	PRIMARY KEY (origin_user_id, target_user_id, target_key_id)
);
`

const selectCrossSigningSigsForTargetSQL = "" +
	"SELECT origin_user_id, origin_key_id, signature FROM keyserver_cross_signing_sigs" +
	" WHERE origin_user_id = $1 AND target_user_id = $2 AND target_key_id = $3"

const upsertCrossSigningSigsForTargetSQL = "" +
	"INSERT OR REPLACE INTO keyserver_cross_signing_sigs (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)" +
	" VALUES($1, $2, $3, $4, $5)"

const deleteCrossSigningSigsForTargetSQL = "" +
	"DELETE FROM keyserver_cross_signing_sigs WHERE target_user_id=$1 AND target_key_id=$2"

type crossSigningSigsStatements struct {
	db                                        *sql.DB
	selectCrossSigningSigsForTargetStmt       *sql.Stmt
	selectCrossSigningSigsForOriginTargetStmt *sql.Stmt
	upsertCrossSigningSigsForTargetStmt       *sql.Stmt
	deleteCrossSigningSigsForTargetStmt       *sql.Stmt
}

func NewSqliteCrossSigningSigsTable(db *sql.DB) (tables.CrossSigningSigs, error) {
	s := &crossSigningSigsStatements{
		db: db,
	}
	_, err := db.Exec(crossSigningSigsSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectCrossSigningSigsForTargetStmt, selectCrossSigningSigsForTargetSQL},
		{&s.upsertCrossSigningSigsForTargetStmt, upsertCrossSigningSigsForTargetSQL},
		{&s.deleteCrossSigningSigsForTargetStmt, deleteCrossSigningSigsForTargetSQL},
	}.Prepare(db)
}

func (s *crossSigningSigsStatements) SelectCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx, originUserID, targetUserID string, targetKeyID gomatrixserverlib.KeyID,
) (r types.CrossSigningSigMap, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectCrossSigningSigsForOriginTargetStmt).QueryContext(ctx, originUserID, targetUserID, targetKeyID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningSigsForOriginTargetStmt: rows.close() failed")
	r = types.CrossSigningSigMap{}
	for rows.Next() {
		var userID string
		var keyID gomatrixserverlib.KeyID
		var signature gomatrixserverlib.Base64Bytes
		if err := rows.Scan(&userID, &keyID, &signature); err != nil {
			return nil, err
		}
		if _, ok := r[userID]; !ok {
			r[userID] = map[gomatrixserverlib.KeyID]gomatrixserverlib.Base64Bytes{}
		}
		r[userID][keyID] = signature
	}
	return
}

func (s *crossSigningSigsStatements) UpsertCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx,
	originUserID string, originKeyID gomatrixserverlib.KeyID,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
	signature gomatrixserverlib.Base64Bytes,
) error {
	if _, err := sqlutil.TxStmt(txn, s.upsertCrossSigningSigsForTargetStmt).ExecContext(ctx, originUserID, originKeyID, targetUserID, targetKeyID, signature); err != nil {
		return fmt.Errorf("s.upsertCrossSigningSigsForTargetStmt: %w", err)
	}
	return nil
}

func (s *crossSigningSigsStatements) DeleteCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
) error {
	if _, err := sqlutil.TxStmt(txn, s.deleteCrossSigningSigsForTargetStmt).ExecContext(ctx, targetUserID, targetKeyID); err != nil {
		return fmt.Errorf("s.deleteCrossSigningSigsForTargetStmt: %w", err)
	}
	return nil
}
