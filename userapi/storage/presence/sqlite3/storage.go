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

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/types"
)

// Database represents an account database
type Database struct {
	db     *sql.DB
	writer sqlutil.Writer
	sqlutil.PartitionOffsetStatements
	presence presenceStatements
}

func NewDatabase(dbProperties *config.DatabaseOptions) (*Database, error) {
	db, err := sqlutil.Open(dbProperties)
	if err != nil {
		return nil, err
	}
	d := &Database{
		db:     db,
		writer: sqlutil.NewExclusiveWriter(),
	}

	if err = d.presence.execSchema(db); err != nil {
		return nil, err
	}
	if err = d.PartitionOffsetStatements.Prepare(db, d.writer, "presence"); err != nil {
		return nil, err
	}
	if err = d.presence.prepare(db); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Database) UpsertPresence(
	ctx context.Context,
	userID, statusMsg string,
	presence types.PresenceStatus,
	lastActiveTS int64,
) (pos int64, err error) {
	err = sqlutil.WithTransaction(d.db, func(txn *sql.Tx) error {
		pos, err = d.presence.UpsertPresence(ctx, txn, userID, statusMsg, presence, lastActiveTS)
		return err
	})
	return
}

func (d *Database) GetPresenceForUser(ctx context.Context, userID string) (api.OutputPresenceData, error) {
	return d.presence.GetPresenceForUser(ctx, nil, userID)
}
