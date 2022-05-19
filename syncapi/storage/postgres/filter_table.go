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

package postgres

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/matrix-org/dendrite/syncapi/storage/tables"
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
	if s.selectFilterStmt, err = db.Prepare(selectFilterSQL); err != nil {
		return nil, err
	}
	if s.selectFilterIDByContentStmt, err = db.Prepare(selectFilterIDByContentSQL); err != nil {
		return nil, err
	}
	if s.insertFilterStmt, err = db.Prepare(insertFilterSQL); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *filterStatements) SelectFilter(
	ctx context.Context, target *gomatrixserverlib.Filter, localpart string, filterID string,
) error {
	// Retrieve filter from database (stored as canonical JSON)
	var filterData []byte
	err := s.selectFilterStmt.QueryRowContext(ctx, localpart, filterID).Scan(&filterData)
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
	ctx context.Context, filter *gomatrixserverlib.Filter, localpart string,
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
