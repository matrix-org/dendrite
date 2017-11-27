// Copyright 2017 Jan Christian Grünhage
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

package accounts

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
)

const selectFilterSQL = "" +
	"SELECT filter FROM account_filter WHERE localpart = $1 AND id = $2"

const selectFilterIDByContentSQL = "" +
	"SELECT id FROM account_filter WHERE localpart = $1 AND filter = $2"

const insertFilterSQL = "" +
	"INSERT INTO account_filter (filter, id, localpart) VALUES ($1, DEFAULT, $2) RETURNING id"

type filterStatements struct {
	selectFilterStmt            *sql.Stmt
	selectFilterIDByContentStmt *sql.Stmt
	insertFilterStmt            *sql.Stmt
}

func (s *filterStatements) prepare(db *sql.DB) (err error) {
	if s.selectFilterStmt, err = db.Prepare(selectFilterSQL); err != nil {
		return
	}
	if s.selectFilterIDByContentStmt, err = db.Prepare(selectFilterIDByContentSQL); err != nil {
		return
	}
	if s.insertFilterStmt, err = db.Prepare(insertFilterSQL); err != nil {
		return
	}
	return
}

func (s *filterStatements) selectFilter(
	ctx context.Context, localpart string, filterID string,
) (filter []byte, err error) {
	err = s.selectFilterStmt.QueryRowContext(ctx, localpart, filterID).Scan(&filter)
	return
}

func (s *filterStatements) insertFilter(
	ctx context.Context, filter []byte, localpart string,
) (filterID string, err error) {
	var existingFilterID string

	// This can result in a race condition when two clients try to insert the
	// same filter and localpart at the same time, however this is not a
	// problem as both calls will result in the same filterID
	filterJSON, err := gomatrixserverlib.CanonicalJSON(filter)
	if err != nil {
		return "", err
	}

	// Check if filter already exists in the database
	err = s.selectFilterIDByContentStmt.QueryRowContext(ctx,
		localpart, filterJSON).Scan(&existingFilterID)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	// If it does, return the existing ID
	if existingFilterID != "" {
		return existingFilterID, err
	}

	// Otherwise insert the filter and return the new ID
	err = s.insertFilterStmt.QueryRowContext(ctx, filterJSON, localpart).
		Scan(&filterID)
	return
}
