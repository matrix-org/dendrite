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
)

const filterSchema = `
-- Stores data about filters
CREATE TABLE IF NOT EXISTS account_filter (
	-- The filter
	filter TEXT NOT NULL,
	-- The ID
	id SERIAL UNIQUE,
	-- The localpart of the Matrix user ID associated to this filter
	localpart TEXT NOT NULL,

	PRIMARY KEY(id, localpart)
);

CREATE INDEX IF NOT EXISTS account_filter_localpart ON account_filter(localpart);
`

const selectFilterSQL = "" +
	"SELECT filter FROM account_filter WHERE localpart = $1 AND id = $2"

const insertFilterSQL = "" +
	"INSERT INTO account_filter (filter, id, localpart) VALUES ($1, DEFAULT, $2) RETURNING id"

type filterStatements struct {
	selectFilterStmt *sql.Stmt
	insertFilterStmt *sql.Stmt
}

func (s *filterStatements) prepare(db *sql.DB) (err error) {
	_, err = db.Exec(filterSchema)
	if err != nil {
		return
	}
	if s.selectFilterStmt, err = db.Prepare(selectFilterSQL); err != nil {
		return
	}
	if s.insertFilterStmt, err = db.Prepare(insertFilterSQL); err != nil {
		return
	}
	return
}

func (s *filterStatements) selectFilter(
	ctx context.Context, localpart string, filterID string,
) (filter string, err error) {
	err = s.selectFilterStmt.QueryRowContext(ctx, localpart, filterID).Scan(&filter)
	return
}

func (s *filterStatements) insertFilter(
	ctx context.Context, filter string, localpart string,
) (pos string, err error) {
	err = s.insertFilterStmt.QueryRowContext(ctx, filter, localpart).Scan(&pos)
	return
}
