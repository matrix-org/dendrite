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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type NotaryID int64

type FederationQueuePDUs interface {
	InsertQueuePDU(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error
	DeleteQueuePDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error
	SelectQueuePDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID int64) (int64, error)
	SelectQueuePDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
	SelectQueuePDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error)
	SelectQueuePDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
}

type FederationQueueEDUs interface {
	InsertQueueEDU(ctx context.Context, txn *sql.Tx, eduType string, serverName gomatrixserverlib.ServerName, nid int64, expiresAt gomatrixserverlib.Timestamp) error
	DeleteQueueEDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error
	SelectQueueEDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error)
	SelectQueueEDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID int64) (int64, error)
	SelectQueueEDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
	SelectQueueEDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
	SelectExpiredEDUs(ctx context.Context, txn *sql.Tx, expiredBefore gomatrixserverlib.Timestamp) ([]int64, error)
	DeleteExpiredEDUs(ctx context.Context, txn *sql.Tx, expiredBefore gomatrixserverlib.Timestamp) error
	Prepare() error
}

type FederationQueueJSON interface {
	InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error)
	DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []int64) error
	SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error)
}

type FederationJoinedHosts interface {
	InsertJoinedHosts(ctx context.Context, txn *sql.Tx, roomID, eventID string, serverName gomatrixserverlib.ServerName) error
	DeleteJoinedHosts(ctx context.Context, txn *sql.Tx, eventIDs []string) error
	DeleteJoinedHostsForRoom(ctx context.Context, txn *sql.Tx, roomID string) error
	SelectJoinedHostsWithTx(ctx context.Context, txn *sql.Tx, roomID string) ([]types.JoinedHost, error)
	SelectJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	SelectAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	SelectJoinedHostsForRooms(ctx context.Context, roomIDs []string, excludingBlacklisted bool) ([]gomatrixserverlib.ServerName, error)
}

type FederationBlacklist interface {
	InsertBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) error
	SelectBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (bool, error)
	DeleteBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) error
	DeleteAllBlacklist(ctx context.Context, txn *sql.Tx) error
}

type FederationOutboundPeeks interface {
	InsertOutboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) (err error)
	RenewOutboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) (err error)
	SelectOutboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string) (outboundPeek *types.OutboundPeek, err error)
	SelectOutboundPeeks(ctx context.Context, txn *sql.Tx, roomID string) (outboundPeeks []types.OutboundPeek, err error)
	DeleteOutboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string) (err error)
	DeleteOutboundPeeks(ctx context.Context, txn *sql.Tx, roomID string) (err error)
}

type FederationInboundPeeks interface {
	InsertInboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) (err error)
	RenewInboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) (err error)
	SelectInboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string) (inboundPeek *types.InboundPeek, err error)
	SelectInboundPeeks(ctx context.Context, txn *sql.Tx, roomID string) (inboundPeeks []types.InboundPeek, err error)
	DeleteInboundPeek(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, roomID, peekID string) (err error)
	DeleteInboundPeeks(ctx context.Context, txn *sql.Tx, roomID string) (err error)
}

// FederationNotaryServerKeysJSON contains the byte-for-byte responses from servers which contain their keys and is signed by them.
type FederationNotaryServerKeysJSON interface {
	// InsertJSONResponse inserts a new response JSON. Useless on its own, needs querying via FederationNotaryServerKeysMetadata
	// `validUntil` should be the value of `valid_until_ts` with the 7-day check applied from:
	//   "Servers MUST use the lesser of this field and 7 days into the future when determining if a key is valid.
	//    This is to avoid a situation where an attacker publishes a key which is valid for a significant amount of time
	//    without a way for the homeserver owner to revoke it.""
	InsertJSONResponse(ctx context.Context, txn *sql.Tx, keyQueryResponseJSON gomatrixserverlib.ServerKeys, serverName gomatrixserverlib.ServerName, validUntil gomatrixserverlib.Timestamp) (NotaryID, error)
}

// FederationNotaryServerKeysMetadata persists the metadata for FederationNotaryServerKeysJSON
type FederationNotaryServerKeysMetadata interface {
	// UpsertKey updates or inserts a (server_name, key_id) tuple, pointing it via NotaryID at the the response which has the longest valid_until_ts
	// `newNotaryID` and `newValidUntil` should be the notary ID / valid_until  which has this (server_name, key_id) tuple already, e.g one you just inserted.
	UpsertKey(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, keyID gomatrixserverlib.KeyID, newNotaryID NotaryID, newValidUntil gomatrixserverlib.Timestamp) (NotaryID, error)
	// SelectKeys returns the signed JSON objects which contain the given key IDs. This will be at most the length of `keyIDs` and at least 1 (assuming
	// the keys exist in the first place). If `keyIDs` is empty, the signed JSON object with the longest valid_until_ts will be returned.
	SelectKeys(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, keyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error)
	// DeleteOldJSONResponses removes all responses which are not referenced in FederationNotaryServerKeysMetadata
	DeleteOldJSONResponses(ctx context.Context, txn *sql.Tx) error
}

type FederationServerSigningKeys interface {
	BulkSelectServerKeys(ctx context.Context, txn *sql.Tx, requests map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.Timestamp) (map[gomatrixserverlib.PublicKeyLookupRequest]gomatrixserverlib.PublicKeyLookupResult, error)
	UpsertServerKeys(ctx context.Context, txn *sql.Tx, request gomatrixserverlib.PublicKeyLookupRequest, key gomatrixserverlib.PublicKeyLookupResult) error
}
