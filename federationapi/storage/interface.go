// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package storage

import (
	"context"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/federationapi/storage/shared/receipt"
	"github.com/element-hq/dendrite/federationapi/types"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
)

type Database interface {
	P2PDatabase
	gomatrixserverlib.KeyDatabase

	UpdateRoom(ctx context.Context, roomID string, addHosts []types.JoinedHost, removeHosts []string, purgeRoomFirst bool) (joinedHosts []types.JoinedHost, err error)

	GetJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	GetAllJoinedHosts(ctx context.Context) ([]spec.ServerName, error)
	// GetJoinedHostsForRooms returns the complete set of servers in the rooms given.
	GetJoinedHostsForRooms(ctx context.Context, roomIDs []string, excludeSelf, excludeBlacklisted bool) ([]spec.ServerName, error)

	StoreJSON(ctx context.Context, js string) (*receipt.Receipt, error)

	GetPendingPDUs(ctx context.Context, serverName spec.ServerName, limit int) (pdus map[*receipt.Receipt]*rstypes.HeaderedEvent, err error)
	GetPendingEDUs(ctx context.Context, serverName spec.ServerName, limit int) (edus map[*receipt.Receipt]*gomatrixserverlib.EDU, err error)

	AssociatePDUWithDestinations(ctx context.Context, destinations map[spec.ServerName]struct{}, dbReceipt *receipt.Receipt) error
	AssociateEDUWithDestinations(ctx context.Context, destinations map[spec.ServerName]struct{}, dbReceipt *receipt.Receipt, eduType string, expireEDUTypes map[string]time.Duration) error

	CleanPDUs(ctx context.Context, serverName spec.ServerName, receipts []*receipt.Receipt) error
	CleanEDUs(ctx context.Context, serverName spec.ServerName, receipts []*receipt.Receipt) error

	GetPendingPDUServerNames(ctx context.Context) ([]spec.ServerName, error)
	GetPendingEDUServerNames(ctx context.Context) ([]spec.ServerName, error)

	// these don't have contexts passed in as we want things to happen regardless of the request context
	AddServerToBlacklist(serverName spec.ServerName) error
	RemoveServerFromBlacklist(serverName spec.ServerName) error
	RemoveAllServersFromBlacklist() error
	IsServerBlacklisted(serverName spec.ServerName) (bool, error)

	// Adds the server to the list of assumed offline servers.
	// If the server already exists in the table, nothing happens and returns success.
	SetServerAssumedOffline(ctx context.Context, serverName spec.ServerName) error
	// Removes the server from the list of assumed offline servers.
	// If the server doesn't exist in the table, nothing happens and returns success.
	RemoveServerAssumedOffline(ctx context.Context, serverName spec.ServerName) error
	// Purges all entries from the assumed offline table.
	RemoveAllServersAssumedOffline(ctx context.Context) error
	// Gets whether the provided server is present in the table.
	// If it is present, returns true. If not, returns false.
	IsServerAssumedOffline(ctx context.Context, serverName spec.ServerName) (bool, error)

	AddOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error
	RenewOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error
	GetOutboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string) (*types.OutboundPeek, error)
	GetOutboundPeeks(ctx context.Context, roomID string) ([]types.OutboundPeek, error)

	AddInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error
	RenewInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string, renewalInterval int64) error
	GetInboundPeek(ctx context.Context, serverName spec.ServerName, roomID, peekID string) (*types.InboundPeek, error)
	GetInboundPeeks(ctx context.Context, roomID string) ([]types.InboundPeek, error)

	// Update the notary with the given server keys from the given server name.
	UpdateNotaryKeys(ctx context.Context, serverName spec.ServerName, serverKeys gomatrixserverlib.ServerKeys) error
	// Query the notary for the server keys for the given server. If `optKeyIDs` is not empty, multiple server keys may be returned (between 1 - len(optKeyIDs))
	// such that the combination of all server keys will include all the `optKeyIDs`.
	GetNotaryKeys(ctx context.Context, serverName spec.ServerName, optKeyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error)
	// DeleteExpiredEDUs cleans up expired EDUs
	DeleteExpiredEDUs(ctx context.Context) error

	PurgeRoom(ctx context.Context, roomID string) error
}

type P2PDatabase interface {
	// Stores the given list of servers as relay servers for the provided destination server.
	// Providing duplicates will only lead to a single entry and won't lead to an error.
	P2PAddRelayServersForServer(ctx context.Context, serverName spec.ServerName, relayServers []spec.ServerName) error

	// Get the list of relay servers associated with the provided destination server.
	// If no entry exists in the table, an empty list is returned and does not result in an error.
	P2PGetRelayServersForServer(ctx context.Context, serverName spec.ServerName) ([]spec.ServerName, error)

	// Deletes any entries for the provided destination server that match the provided relayServers list.
	// If any of the provided servers don't match an entry, nothing happens and no error is returned.
	P2PRemoveRelayServersForServer(ctx context.Context, serverName spec.ServerName, relayServers []spec.ServerName) error

	// Deletes all entries for the provided destination server.
	// If the destination server doesn't exist in the table, nothing happens and no error is returned.
	P2PRemoveAllRelayServersForServer(ctx context.Context, serverName spec.ServerName) error
}
