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
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// Database stores information needed by the federation sender
type Database struct {
	joinedHostsStatements
	roomStatements
	queueStatements
	queueJSONStatements
	sqlutil.PartitionOffsetStatements
	db *sql.DB
}

// NewDatabase opens a new database
func NewDatabase(dataSourceName string, dbProperties sqlutil.DbProperties) (*Database, error) {
	var result Database
	var err error
	if result.db, err = sqlutil.Open("postgres", dataSourceName, dbProperties); err != nil {
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

	if err = d.queueStatements.prepare(d.db); err != nil {
		return err
	}

	if err = d.queueJSONStatements.prepare(d.db); err != nil {
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

// StoreJSON adds a JSON blob into the queue JSON table and returns
// a NID. The NID will then be used when inserting the per-destination
// metadata entries.
func (d *Database) StoreJSON(
	ctx context.Context, js []byte,
) (int64, error) {
	res, err := d.insertJSONStmt.ExecContext(ctx, js)
	if err != nil {
		return 0, fmt.Errorf("d.insertRetryJSONStmt: %w", err)
	}
	nid, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("res.LastInsertID: %w", err)
	}
	return nid, nil
}

// AssociatePDUWithDestination creates an association that the
// destination queues will use to determine which JSON blobs to send
// to which servers.
func (d *Database) AssociatePDUWithDestination(
	ctx context.Context,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	nids []int64,
) error {
	for _, nid := range nids {
		if err := d.insertQueuePDU(
			ctx,           // context
			nil,           // SQL transaction
			transactionID, // transaction ID
			serverName,    // destination server name
			nid,           // NID from the federationsender_queue_json table
		); err != nil {
			return fmt.Errorf("d.insertQueueRetryStmt.ExecContext: %w", err)
		}
	}
	return nil
}

// GetNextTransactionPDUs retrieves events from the database for
// the next pending transaction, up to the limit specified.
func (d *Database) GetNextTransactionPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (gomatrixserverlib.TransactionID, []*gomatrixserverlib.HeaderedEvent, error) {
	transactionID, err := d.selectQueueNextTransactionID(ctx, nil, string(serverName), types.FailedEventTypePDU)
	if err != nil {
		return "", nil, fmt.Errorf("d.selectRetryNextTransactionID: %w", err)
	}

	nids, err := d.selectQueuePDUs(ctx, nil, string(serverName), transactionID, limit)
	if err != nil {
		return "", nil, fmt.Errorf("d.selectQueueRetryPDUs: %w", err)
	}

	blobs, err := d.selectJSON(ctx, nil, nids)
	if err != nil {
		return "", nil, fmt.Errorf("d.selectJSON: %w", err)
	}

	var events []*gomatrixserverlib.HeaderedEvent
	for _, blob := range blobs {
		var event gomatrixserverlib.HeaderedEvent
		if err := json.Unmarshal(blob, &event); err != nil {
			return "", nil, fmt.Errorf("json.Unmarshal: %w", err)
		}
		events = append(events, &event)
	}

	return gomatrixserverlib.TransactionID(transactionID), events, nil
}

// CleanTransactionPDUs cleans up all associated events for a
// given transaction. This is done when the transaction was sent
// successfully.
func (d *Database) CleanTransactionPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	transactionID gomatrixserverlib.TransactionID,
) error {
	return d.deleteQueueTransaction(ctx, nil, serverName, transactionID)
}
