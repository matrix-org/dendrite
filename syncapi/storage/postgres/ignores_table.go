// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
)

const ignoresSchema = `
-- Stores data about ignoress
CREATE TABLE IF NOT EXISTS syncapi_ignores (
	-- The user ID whose ignore list this belongs to.
	user_id TEXT NOT NULL,
	ignores_json TEXT NOT NULL,
	PRIMARY KEY(user_id)
);
`

const selectIgnoresSQL = "" +
	"SELECT ignores_json FROM syncapi_ignores WHERE user_id = $1"

const upsertIgnoresSQL = "" +
	"INSERT INTO syncapi_ignores (user_id, ignores_json) VALUES ($1, $2)" +
	" ON CONFLICT (user_id) DO UPDATE set ignores_json = $2"

type ignoresStatements struct {
	selectIgnoresStmt *sql.Stmt
	upsertIgnoresStmt *sql.Stmt
}

func NewPostgresIgnoresTable(db *sql.DB) (tables.Ignores, error) {
	_, err := db.Exec(ignoresSchema)
	if err != nil {
		return nil, err
	}
	s := &ignoresStatements{}

	return s, sqlutil.StatementList{
		{&s.selectIgnoresStmt, selectIgnoresSQL},
		{&s.upsertIgnoresStmt, upsertIgnoresSQL},
	}.Prepare(db)
}

func (s *ignoresStatements) SelectIgnores(
	ctx context.Context, txn *sql.Tx, userID string,
) (*types.IgnoredUsers, error) {
	var ignoresData []byte
	err := sqlutil.TxStmt(txn, s.selectIgnoresStmt).QueryRowContext(ctx, userID).Scan(&ignoresData)
	if err != nil {
		return nil, err
	}
	var ignores types.IgnoredUsers
	if err = json.Unmarshal(ignoresData, &ignores); err != nil {
		return nil, err
	}
	return &ignores, nil
}

func (s *ignoresStatements) UpsertIgnores(
	ctx context.Context, txn *sql.Tx, userID string, ignores *types.IgnoredUsers,
) error {
	ignoresJSON, err := json.Marshal(ignores)
	if err != nil {
		return err
	}
	_, err = sqlutil.TxStmt(txn, s.upsertIgnoresStmt).ExecContext(ctx, userID, ignoresJSON)
	return err
}
