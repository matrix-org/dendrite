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
	"fmt"
	"time"

	"github.com/matrix-org/dendrite/federationsender/storage/tables"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                            *sql.DB
	Cache                         caching.FederationSenderCache
	Writer                        sqlutil.Writer
	FederationSenderQueuePDUs     tables.FederationSenderQueuePDUs
	FederationSenderQueueEDUs     tables.FederationSenderQueueEDUs
	FederationSenderQueueJSON     tables.FederationSenderQueueJSON
	FederationSenderJoinedHosts   tables.FederationSenderJoinedHosts
	FederationSenderBlacklist     tables.FederationSenderBlacklist
	FederationSenderOutboundPeeks tables.FederationSenderOutboundPeeks
	FederationSenderInboundPeeks  tables.FederationSenderInboundPeeks
	NotaryServerKeysJSON          tables.FederationSenderNotaryServerKeysJSON
	NotaryServerKeysMetadata      tables.FederationSenderNotaryServerKeysMetadata
}

// An Receipt contains the NIDs of a call to GetNextTransactionPDUs/EDUs.
// We don't actually export the NIDs but we need the caller to be able
// to pass them back so that we can clean up if the transaction sends
// successfully.
type Receipt struct {
	nid int64
}

func (r *Receipt) String() string {
	return fmt.Sprintf("%d", r.nid)
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
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
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
		return nil
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
	var nid int64
	var err error
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nid, err = d.FederationSenderQueueJSON.InsertQueueJSON(ctx, txn, js)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("d.insertQueueJSON: %w", err)
	}
	return &Receipt{
		nid: nid,
	}, nil
}

func (d *Database) PurgeRoomState(
	ctx context.Context, roomID string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		// If the event is a create event then we'll delete all of the existing
		// data for the room. The only reason that a create event would be replayed
		// to us in this way is if we're about to receive the entire room state.
		if err := d.FederationSenderJoinedHosts.DeleteJoinedHostsForRoom(ctx, txn, roomID); err != nil {
			return fmt.Errorf("d.FederationSenderJoinedHosts.DeleteJoinedHosts: %w", err)
		}
		return nil
	})
}

func (d *Database) AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderBlacklist.InsertBlacklist(context.TODO(), txn, serverName)
	})
}

func (d *Database) RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderBlacklist.DeleteBlacklist(context.TODO(), txn, serverName)
	})
}

func (d *Database) RemoveAllServersFromBlacklist() error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderBlacklist.DeleteAllBlacklist(context.TODO(), txn)
	})
}

func (d *Database) IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error) {
	return d.FederationSenderBlacklist.SelectBlacklist(context.TODO(), nil, serverName)
}

func (d *Database) AddOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderOutboundPeeks.InsertOutboundPeek(ctx, txn, serverName, roomID, peekID, renewalInterval)
	})
}

func (d *Database) RenewOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderOutboundPeeks.RenewOutboundPeek(ctx, txn, serverName, roomID, peekID, renewalInterval)
	})
}

func (d *Database) GetOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.OutboundPeek, error) {
	return d.FederationSenderOutboundPeeks.SelectOutboundPeek(ctx, nil, serverName, roomID, peekID)
}

func (d *Database) GetOutboundPeeks(ctx context.Context, roomID string) ([]types.OutboundPeek, error) {
	return d.FederationSenderOutboundPeeks.SelectOutboundPeeks(ctx, nil, roomID)
}

func (d *Database) AddInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderInboundPeeks.InsertInboundPeek(ctx, txn, serverName, roomID, peekID, renewalInterval)
	})
}

func (d *Database) RenewInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.FederationSenderInboundPeeks.RenewInboundPeek(ctx, txn, serverName, roomID, peekID, renewalInterval)
	})
}

func (d *Database) GetInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.InboundPeek, error) {
	return d.FederationSenderInboundPeeks.SelectInboundPeek(ctx, nil, serverName, roomID, peekID)
}

func (d *Database) GetInboundPeeks(ctx context.Context, roomID string) ([]types.InboundPeek, error) {
	return d.FederationSenderInboundPeeks.SelectInboundPeeks(ctx, nil, roomID)
}

func (d *Database) UpdateNotaryKeys(ctx context.Context, serverName gomatrixserverlib.ServerName, serverKeys gomatrixserverlib.ServerKeys) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		validUntil := serverKeys.ValidUntilTS
		// Servers MUST use the lesser of this field and 7 days into the future when determining if a key is valid.
		// This is to avoid a situation where an attacker publishes a key which is valid for a significant amount of
		// time without a way for the homeserver owner to revoke it.
		// https://spec.matrix.org/unstable/server-server-api/#querying-keys-through-another-server
		weekIntoFuture := time.Now().Add(7 * 24 * time.Hour)
		if weekIntoFuture.Before(validUntil.Time()) {
			validUntil = gomatrixserverlib.AsTimestamp(weekIntoFuture)
		}
		notaryID, err := d.NotaryServerKeysJSON.InsertJSONResponse(ctx, txn, serverKeys, serverName, validUntil)
		if err != nil {
			return err
		}
		// update the metadata for the keys
		for keyID := range serverKeys.OldVerifyKeys {
			_, err = d.NotaryServerKeysMetadata.UpsertKey(ctx, txn, serverName, keyID, notaryID, validUntil)
			if err != nil {
				return err
			}
		}
		for keyID := range serverKeys.VerifyKeys {
			_, err = d.NotaryServerKeysMetadata.UpsertKey(ctx, txn, serverName, keyID, notaryID, validUntil)
			if err != nil {
				return err
			}
		}

		// clean up old responses
		return d.NotaryServerKeysMetadata.DeleteOldJSONResponses(ctx, txn)
	})
}

func (d *Database) GetNotaryKeys(
	ctx context.Context, serverName gomatrixserverlib.ServerName, optKeyIDs []gomatrixserverlib.KeyID,
) (sks []gomatrixserverlib.ServerKeys, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sks, err = d.NotaryServerKeysMetadata.SelectKeys(ctx, txn, serverName, optKeyIDs)
		return err
	})
	return sks, err
}
