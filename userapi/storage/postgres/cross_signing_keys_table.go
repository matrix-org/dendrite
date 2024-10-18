// Copyright 2024 New Vector Ltd.
// Copyright 2021 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/element-hq/dendrite/internal"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	"github.com/element-hq/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var crossSigningKeysSchema = `
CREATE TABLE IF NOT EXISTS keyserver_cross_signing_keys (
    user_id TEXT NOT NULL,
	key_type SMALLINT NOT NULL,
	key_data TEXT NOT NULL,
	PRIMARY KEY (user_id, key_type)
);
`

const selectCrossSigningKeysForUserSQL = "" +
	"SELECT key_type, key_data FROM keyserver_cross_signing_keys" +
	" WHERE user_id = $1"

const upsertCrossSigningKeysForUserSQL = "" +
	"INSERT INTO keyserver_cross_signing_keys (user_id, key_type, key_data)" +
	" VALUES($1, $2, $3)" +
	" ON CONFLICT (user_id, key_type) DO UPDATE SET key_data = $3"

type crossSigningKeysStatements struct {
	db                                *sql.DB
	selectCrossSigningKeysForUserStmt *sql.Stmt
	upsertCrossSigningKeysForUserStmt *sql.Stmt
}

func NewPostgresCrossSigningKeysTable(db *sql.DB) (tables.CrossSigningKeys, error) {
	s := &crossSigningKeysStatements{
		db: db,
	}
	_, err := db.Exec(crossSigningKeysSchema)
	if err != nil {
		return nil, err
	}
	return s, sqlutil.StatementList{
		{&s.selectCrossSigningKeysForUserStmt, selectCrossSigningKeysForUserSQL},
		{&s.upsertCrossSigningKeysForUserStmt, upsertCrossSigningKeysForUserSQL},
	}.Prepare(db)
}

func (s *crossSigningKeysStatements) SelectCrossSigningKeysForUser(
	ctx context.Context, txn *sql.Tx, userID string,
) (r types.CrossSigningKeyMap, err error) {
	rows, err := sqlutil.TxStmt(txn, s.selectCrossSigningKeysForUserStmt).QueryContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningKeysForUserStmt: rows.close() failed")
	r = types.CrossSigningKeyMap{}
	for rows.Next() {
		var keyTypeInt int16
		var keyData spec.Base64Bytes
		if err = rows.Scan(&keyTypeInt, &keyData); err != nil {
			return nil, err
		}
		keyType, ok := types.KeyTypeIntToPurpose[keyTypeInt]
		if !ok {
			return nil, fmt.Errorf("unknown key purpose int %d", keyTypeInt)
		}
		r[keyType] = keyData
	}
	err = rows.Err()
	return
}

func (s *crossSigningKeysStatements) UpsertCrossSigningKeysForUser(
	ctx context.Context, txn *sql.Tx, userID string, keyType fclient.CrossSigningKeyPurpose, keyData spec.Base64Bytes,
) error {
	keyTypeInt, ok := types.KeyTypePurposeToInt[keyType]
	if !ok {
		return fmt.Errorf("unknown key purpose %q", keyType)
	}
	if _, err := sqlutil.TxStmt(txn, s.upsertCrossSigningKeysForUserStmt).ExecContext(ctx, userID, keyTypeInt, keyData); err != nil {
		return fmt.Errorf("s.upsertCrossSigningKeysForUserStmt: %w", err)
	}
	return nil
}
