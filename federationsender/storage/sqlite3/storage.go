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

package sqlite3

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database stores information needed by the federation sender
type Database struct {
	joinedHostsStatements
	roomStatements
	queueRetryStatements
	sqlutil.PartitionOffsetStatements
	db *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string) (*Database, error) {
	var result Database
	var err error
	cs, err := sqlutil.ParseFileURI(dataSourceName)
	if err != nil {
		return nil, err
	}
	if result.db, err = sqlutil.Open(sqlutil.SQLiteDriverName(), cs, nil); err != nil {
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

	if err = d.queueRetryStatements.prepare(d.db); err != nil {
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
	err = sqlutil.WithTransaction(d.db, func(txn *sql.Tx) error {
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

		if lastSentEventID != "" && lastSentEventID != oldEventID {
			return types.EventIDMismatchError{
				DatabaseID: lastSentEventID, RoomServerID: oldEventID,
			}
		}

		joinedHosts, err = d.selectJoinedHostsWithTx(ctx, txn, roomID)
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

// GetJoinedHosts returns the currently joined hosts for room,
// as known to federationserver.
// Returns an error if something goes wrong.
func (d *Database) GetJoinedHosts(
	ctx context.Context, roomID string,
) ([]types.JoinedHost, error) {
	return d.selectJoinedHosts(ctx, roomID)
}

// GetFailedPDUs retrieves PDUs that we have failed to send on
// a specific destination queue.
func (d *Database) GetFailedPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) ([]*gomatrixserverlib.HeaderedEvent, error) {
	transactionID, err := d.selectRetryNextTransactionID(ctx, nil, string(serverName), types.FailedEventTypePDU)
	if err != nil {
		return nil, fmt.Errorf("d.selectRetryNextTransactionID: %w", err)
	}

	events, err := d.selectQueueRetryPDUs(ctx, nil, string(serverName), transactionID)
	if err != nil {
		return nil, fmt.Errorf("d.selectQueueRetryPDUs: %w", err)
	}
	return events, nil
}

// StoreFailedPDUs stores PDUs that we have failed to send on
// a specific destination queue.
func (d *Database) StoreFailedPDUs(
	ctx context.Context,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	pdus []*gomatrixserverlib.HeaderedEvent,
) error {
	for _, pdu := range pdus {
		if _, err := d.insertRetryStmt.ExecContext(
			ctx,
			string(transactionID),    // transaction ID
			types.FailedEventTypePDU, // type of event that was queued
			pdu.EventID(),            // event ID
			pdu.OriginServerTS(),     // event origin server TS
			string(serverName),       // destination server name
			pdu.JSON(),               // JSON body
		); err != nil {
			return fmt.Errorf("d.insertQueueRetryStmt.ExecContext: %w", err)
		}
	}
	return nil
}
