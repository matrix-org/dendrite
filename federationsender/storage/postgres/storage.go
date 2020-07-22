// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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
	"database/sql"

	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/internal/sqlutil"
)

// Database stores information needed by the federation sender
type Database struct {
	shared.Database
	sqlutil.PartitionOffsetStatements
	db *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string, dbProperties sqlutil.DbProperties) (*Database, error) {
	var d Database
	var err error
	if d.db, err = sqlutil.Open("postgres", dataSourceName, dbProperties); err != nil {
		return nil, err
	}
	joinedHosts, err := NewPostgresJoinedHostsTable(d.db)
	if err != nil {
		return nil, err
	}
	queuePDUs, err := NewPostgresQueuePDUsTable(d.db)
	if err != nil {
		return nil, err
	}
	queueEDUs, err := NewPostgresQueueEDUsTable(d.db)
	if err != nil {
		return nil, err
	}
	queueJSON, err := NewPostgresQueueJSONTable(d.db)
	if err != nil {
		return nil, err
	}
	rooms, err := NewPostgresRoomsTable(d.db)
	if err != nil {
		return nil, err
	}
	blacklist, err := NewPostgresBlacklistTable(d.db)
	if err != nil {
		return nil, err
	}
	d.Database = shared.Database{
		DB:                          d.db,
		FederationSenderJoinedHosts: joinedHosts,
		FederationSenderQueuePDUs:   queuePDUs,
		FederationSenderQueueEDUs:   queueEDUs,
		FederationSenderQueueJSON:   queueJSON,
		FederationSenderRooms:       rooms,
		FederationSenderBlacklist:   blacklist,
	}
	if err = d.PartitionOffsetStatements.Prepare(d.db, "federationsender"); err != nil {
		return nil, err
	}
	return &d, nil
}
