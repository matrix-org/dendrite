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
	"github.com/element-hq/dendrite/userapi/storage/postgres/deltas"
	"github.com/element-hq/dendrite/userapi/storage/tables"
	"github.com/element-hq/dendrite/userapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

var crossSigningSigsSchema = `
CREATE TABLE IF NOT EXISTS keyserver_cross_signing_sigs (
    origin_user_id TEXT NOT NULL,
	origin_key_id TEXT NOT NULL,
	target_user_id TEXT NOT NULL,
	target_key_id TEXT NOT NULL,
	signature TEXT NOT NULL,
	PRIMARY KEY (origin_user_id, origin_key_id, target_user_id, target_key_id)
);

CREATE INDEX IF NOT EXISTS keyserver_cross_signing_sigs_idx ON keyserver_cross_signing_sigs (origin_user_id, target_user_id, target_key_id);
`

const selectCrossSigningSigsForTargetSQL = "" +
	"SELECT origin_user_id, origin_key_id, signature FROM keyserver_cross_signing_sigs" +
	" WHERE (origin_user_id = $1 OR origin_user_id = $2) AND target_user_id = $2 AND target_key_id = $3"

const upsertCrossSigningSigsForTargetSQL = "" +
	"INSERT INTO keyserver_cross_signing_sigs (origin_user_id, origin_key_id, target_user_id, target_key_id, signature)" +
	" VALUES($1, $2, $3, $4, $5)" +
	" ON CONFLICT (origin_user_id, origin_key_id, target_user_id, target_key_id) DO UPDATE SET signature = $5"

const deleteCrossSigningSigsForTargetSQL = "" +
	"DELETE FROM keyserver_cross_signing_sigs WHERE target_user_id=$1 AND target_key_id=$2"

type crossSigningSigsStatements struct {
	db                                  *sql.DB
	selectCrossSigningSigsForTargetStmt *sql.Stmt
	upsertCrossSigningSigsForTargetStmt *sql.Stmt
	deleteCrossSigningSigsForTargetStmt *sql.Stmt
}

func NewPostgresCrossSigningSigsTable(db *sql.DB) (tables.CrossSigningSigs, error) {
	s := &crossSigningSigsStatements{
		db: db,
	}
	_, err := db.Exec(crossSigningSigsSchema)
	if err != nil {
		return nil, err
	}

	m := sqlutil.NewMigrator(db)
	m.AddMigrations(sqlutil.Migration{
		Version: "keyserver: cross signing signature indexes",
		Up:      deltas.UpFixCrossSigningSignatureIndexes,
	})
	if err = m.Up(context.Background()); err != nil {
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
	rows, err := sqlutil.TxStmt(txn, s.selectCrossSigningSigsForTargetStmt).QueryContext(ctx, originUserID, targetUserID, targetKeyID)
	if err != nil {
		return nil, err
	}
	defer internal.CloseAndLogIfError(ctx, rows, "selectCrossSigningSigsForTargetStmt: rows.close() failed")
	r = types.CrossSigningSigMap{}
	for rows.Next() {
		var userID string
		var keyID gomatrixserverlib.KeyID
		var signature spec.Base64Bytes
		if err = rows.Scan(&userID, &keyID, &signature); err != nil {
			return nil, err
		}
		if _, ok := r[userID]; !ok {
			r[userID] = map[gomatrixserverlib.KeyID]spec.Base64Bytes{}
		}
		r[userID][keyID] = signature
	}
	err = rows.Err()
	return
}

func (s *crossSigningSigsStatements) UpsertCrossSigningSigsForTarget(
	ctx context.Context, txn *sql.Tx,
	originUserID string, originKeyID gomatrixserverlib.KeyID,
	targetUserID string, targetKeyID gomatrixserverlib.KeyID,
	signature spec.Base64Bytes,
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
