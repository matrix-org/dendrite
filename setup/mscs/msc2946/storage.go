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

package msc2946

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	StoreRelation(ctx context.Context, he *gomatrixserverlib.HeaderedEvent) error
}

type DB struct {
	db             *sql.DB
	writer         sqlutil.Writer
	insertEdgeStmt *sql.Stmt
}

// NewDatabase loads the database for msc2836
func NewDatabase(dbOpts *config.DatabaseOptions) (Database, error) {
	return &DB{}, nil
}

func (d *DB) StoreRelation(ctx context.Context, he *gomatrixserverlib.HeaderedEvent) error {
	return nil
}
