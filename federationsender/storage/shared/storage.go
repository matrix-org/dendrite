// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/federationsender/storage/tables"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                          *sql.DB
	FederationSenderQueuePDUs   tables.FederationSenderQueuePDUs
	FederationSenderQueueEDUs   tables.FederationSenderQueueEDUs
	FederationSenderQueueJSON   tables.FederationSenderQueueJSON
	FederationSenderJoinedHosts tables.FederationSenderJoinedHosts
	FederationSenderRooms       tables.FederationSenderRooms
	FederationSenderBlacklist   tables.FederationSenderBlacklist
}

// An Receipt contains the NIDs of a call to GetNextTransactionPDUs/EDUs.
// We don't actually export the NIDs but we need the caller to be able
// to pass them back so that we can clean up if the transaction sends
// successfully.
type Receipt struct {
	nids []int64
}

func (e *Receipt) Empty() bool {
	return len(e.nids) == 0
}

func (e *Receipt) String() string {
	j, _ := json.Marshal(e.nids)
	return string(j)
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
	err = sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		err = d.FederationSenderRooms.InsertRoom(ctx, txn, roomID)
		if err != nil {
			return err
		}

		lastSentEventID, err := d.FederationSenderRooms.SelectRoomForUpdate(ctx, txn, roomID)
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

		joinedHosts, err = d.FederationSenderJoinedHosts.SelectJoinedHostsWithTx(ctx, txn, roomID)
		if err != nil {
			return err
		}

		for _, add := range addHosts {
			err = d.FederationSenderJoinedHosts.InsertJoinedHosts(ctx, txn, roomID, add.MemberEventID, add.ServerName)
			if err != nil {
				return err
			}
		}
		if err = d.FederationSenderJoinedHosts.DeleteJoinedHosts(ctx, txn, removeHosts); err != nil {
			return err
		}
		return d.FederationSenderRooms.UpdateRoom(ctx, txn, roomID, newEventID)
	})
	return
}

// GetJoinedHosts returns the currently joined hosts for room,
// as known to federationserver.
// Returns an error if something goes wrong.
func (d *Database) GetJoinedHosts(
	ctx context.Context, roomID string,
) ([]types.JoinedHost, error) {
	return d.FederationSenderJoinedHosts.SelectJoinedHosts(ctx, roomID)
}

// GetAllJoinedHosts returns the currently joined hosts for
// all rooms known to the federation sender.
// Returns an error if something goes wrong.
func (d *Database) GetAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationSenderJoinedHosts.SelectAllJoinedHosts(ctx)
}

func (d *Database) GetJoinedHostsForRooms(ctx context.Context, roomIDs []string) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationSenderJoinedHosts.SelectJoinedHostsForRooms(ctx, roomIDs)
}

// StoreJSON adds a JSON blob into the queue JSON table and returns
// a NID. The NID will then be used when inserting the per-destination
// metadata entries.
func (d *Database) StoreJSON(
	ctx context.Context, js string,
) (*Receipt, error) {
	nid, err := d.FederationSenderQueueJSON.InsertQueueJSON(ctx, nil, js)
	if err != nil {
		return nil, fmt.Errorf("d.insertQueueJSON: %w", err)
	}
	return &Receipt{
		nids: []int64{nid},
	}, nil
}

func (d *Database) AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error {
	return d.FederationSenderBlacklist.InsertBlacklist(context.TODO(), nil, serverName)
}

func (d *Database) RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error {
	return d.FederationSenderBlacklist.DeleteBlacklist(context.TODO(), nil, serverName)
}

func (d *Database) IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error) {
	return d.FederationSenderBlacklist.SelectBlacklist(context.TODO(), nil, serverName)
}
