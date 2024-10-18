// Copyright 2024 New Vector Ltd.
// Copyright 2017 Jan Christian Grünhage
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
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/matrix-org/gomatrixserverlib"
)

const filterSchema = `
-- Stores data about filters
CREATE TABLE IF NOT EXISTS syncapi_filter (
	-- The filter
	filter TEXT NOT NULL,
	-- The ID
	id SERIAL UNIQUE,
	-- The localpart of the Matrix user ID associated to this filter
	localpart TEXT NOT NULL,

	PRIMARY KEY(id, localpart)
);

CREATE INDEX IF NOT EXISTS syncapi_filter_localpart ON syncapi_filter(localpart);
`

const selectFilterSQL = "" +
	"SELECT filter FROM syncapi_filter WHERE localpart = $1 AND id = $2"

const selectFilterIDByContentSQL = "" +
	"SELECT id FROM syncapi_filter WHERE localpart = $1 AND filter = $2"

const insertFilterSQL = "" +
	"INSERT INTO syncapi_filter (filter, id, localpart) VALUES ($1, DEFAULT, $2) RETURNING id"

type filterStatements struct {
	selectFilterStmt            *sql.Stmt
	selectFilterIDByContentStmt *sql.Stmt
	insertFilterStmt            *sql.Stmt
}

func NewPostgresFilterTable(db *sql.DB) (tables.Filter, error) {
	_, err := db.Exec(filterSchema)
	if err != nil {
		return nil, err
	}
	s := &filterStatements{}
	return s, sqlutil.StatementList{
		{&s.selectFilterStmt, selectFilterSQL},
		{&s.selectFilterIDByContentStmt, selectFilterIDByContentSQL},
		{&s.insertFilterStmt, insertFilterSQL},
	}.Prepare(db)
}

func (s *filterStatements) SelectFilter(
	ctx context.Context, txn *sql.Tx, target *synctypes.Filter, localpart string, filterID string,
) error {
	// Retrieve filter from database (stored as canonical JSON)
	var filterData []byte
	err := sqlutil.TxStmt(txn, s.selectFilterStmt).QueryRowContext(ctx, localpart, filterID).Scan(&filterData)
	if err != nil {
		return err
	}

	// Unmarshal JSON into Filter struct
	if err = json.Unmarshal(filterData, &target); err != nil {
		return err
	}
	return nil
}

func (s *filterStatements) InsertFilter(
	ctx context.Context, txn *sql.Tx, filter *synctypes.Filter, localpart string,
) (filterID string, err error) {
	var existingFilterID string

	// Serialise json
	filterJSON, err := json.Marshal(filter)
	if err != nil {
		return "", err
	}
	// Remove whitespaces and sort JSON data
	// needed to prevent from inserting the same filter multiple times
	filterJSON, err = gomatrixserverlib.CanonicalJSON(filterJSON)
	if err != nil {
		return "", err
	}

	// Check if filter already exists in the database using its localpart and content
	//
	// This can result in a race condition when two clients try to insert the
	// same filter and localpart at the same time, however this is not a
	// problem as both calls will result in the same filterID
	err = sqlutil.TxStmt(txn, s.selectFilterIDByContentStmt).QueryRowContext(
		ctx, localpart, filterJSON,
	).Scan(&existingFilterID)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	// If it does, return the existing ID
	if existingFilterID != "" {
		return existingFilterID, err
	}

	// Otherwise insert the filter and return the new ID
	err = sqlutil.TxStmt(txn, s.insertFilterStmt).QueryRowContext(ctx, filterJSON, localpart).
		Scan(&filterID)
	return
}
