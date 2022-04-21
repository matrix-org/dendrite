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

package storage

import (
	"context"

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	gomatrixserverlib.KeyDatabase

	UpdateRoom(ctx context.Context, roomID, oldEventID, newEventID string, addHosts []types.JoinedHost, removeHosts []string) (joinedHosts []types.JoinedHost, err error)

	GetJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	GetAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	// GetJoinedHostsForRooms returns the complete set of servers in the rooms given.
	GetJoinedHostsForRooms(ctx context.Context, roomIDs []string, excludeSelf bool) ([]gomatrixserverlib.ServerName, error)
	PurgeRoomState(ctx context.Context, roomID string) error

	StoreJSON(ctx context.Context, js string) (*shared.Receipt, error)

	GetPendingPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (pdus map[*shared.Receipt]*gomatrixserverlib.HeaderedEvent, err error)
	GetPendingEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (edus map[*shared.Receipt]*gomatrixserverlib.EDU, err error)

	AssociatePDUWithDestination(ctx context.Context, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error
	AssociateEDUWithDestination(ctx context.Context, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt, eduType string) error

	CleanPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error
	CleanEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipts []*shared.Receipt) error

	GetPendingPDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error)
	GetPendingEDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error)

	GetPendingPDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	GetPendingEDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error)

	// these don't have contexts passed in as we want things to happen regardless of the request context
	AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error
	RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error
	RemoveAllServersFromBlacklist() error
	IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error)

	AddOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error
	RenewOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error
	GetOutboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.OutboundPeek, error)
	GetOutboundPeeks(ctx context.Context, roomID string) ([]types.OutboundPeek, error)

	AddInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error
	RenewInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string, renewalInterval int64) error
	GetInboundPeek(ctx context.Context, serverName gomatrixserverlib.ServerName, roomID, peekID string) (*types.InboundPeek, error)
	GetInboundPeeks(ctx context.Context, roomID string) ([]types.InboundPeek, error)

	// Update the notary with the given server keys from the given server name.
	UpdateNotaryKeys(ctx context.Context, serverName gomatrixserverlib.ServerName, serverKeys gomatrixserverlib.ServerKeys) error
	// Query the notary for the server keys for the given server. If `optKeyIDs` is not empty, multiple server keys may be returned (between 1 - len(optKeyIDs))
	// such that the combination of all server keys will include all the `optKeyIDs`.
	GetNotaryKeys(ctx context.Context, serverName gomatrixserverlib.ServerName, optKeyIDs []gomatrixserverlib.KeyID) ([]gomatrixserverlib.ServerKeys, error)
}
