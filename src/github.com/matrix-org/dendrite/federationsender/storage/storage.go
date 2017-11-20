// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/types"
)

// Database stores information needed by the federation sender
type Database struct {
	joinedHostsStatements
	roomStatements
	common.PartitionOffsetStatements
	db *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string) (*Database, error) {
	var result Database
	var err error
	if result.db, err = sql.Open("postgres", dataSourceName); err != nil {
		return nil, err
	}
	if err = result.prepare(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (d *Database) prepare() error {
	var err error

	if err = d.joinedHostsStatements.prepare(d.db); err != nil {
		return err
	}

	if err = d.roomStatements.prepare(d.db); err != nil {
		return err
	}

	return d.PartitionOffsetStatements.Prepare(d.db, "federationsender")
}

// UpdateRoom updates the joined hosts for a room and returns what the joined
// hosts were before the update, or nil if this was a duplicate message.
// This is called when we receive a message from kafka, so we pass in
// oldEventID and newEventID to check that we haven't missed any messages or
// this isn't a duplicate message.
func (d *Database) UpdateRoom(
	ctx context.Context,
	roomID, oldEventID, newEventID string,
	addHosts []types.JoinedHost,
	removeHosts []string,
) (joinedHosts []types.JoinedHost, err error) {
	err = common.WithTransaction(d.db, func(txn *sql.Tx) error {
		err = d.insertRoom(ctx, txn, roomID)
		if err != nil {
			return err
		}

		lastSentEventID, err := d.selectRoomForUpdate(ctx, txn, roomID)
		if err != nil {
			return err
		}

		if lastSentEventID == newEventID {
			// We've handled this message before, so let's just ignore it.
			// We can only get a duplicate for the last message we processed,
			// so its enough just to compare the newEventID with lastSentEventID
			return nil
		}

		if lastSentEventID != oldEventID {
			return types.EventIDMismatchError{
				DatabaseID: lastSentEventID, RoomServerID: oldEventID,
			}
		}

		joinedHosts, err = d.selectJoinedHosts(ctx, txn, roomID)
		if err != nil {
			return err
		}

		for _, add := range addHosts {
			err = d.insertJoinedHosts(ctx, txn, roomID, add.MemberEventID, add.ServerName)
			if err != nil {
				return err
			}
		}
		if err = d.deleteJoinedHosts(ctx, txn, removeHosts); err != nil {
			return err
		}
		return d.updateRoom(ctx, txn, roomID, newEventID)
	})
	return
}
