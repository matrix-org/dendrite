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

	"github.com/matrix-org/dendrite/federationsender/storage/shared"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	internal.PartitionStorer

	UpdateRoom(ctx context.Context, roomID, oldEventID, newEventID string, addHosts []types.JoinedHost, removeHosts []string) (joinedHosts []types.JoinedHost, err error)

	GetJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	GetAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	// GetJoinedHostsForRooms returns the complete set of servers in the rooms given.
	GetJoinedHostsForRooms(ctx context.Context, roomIDs []string) ([]gomatrixserverlib.ServerName, error)
	PurgeRoomState(ctx context.Context, roomID string) error

	StoreJSON(ctx context.Context, js string) (*shared.Receipt, error)

	AssociatePDUWithDestination(ctx context.Context, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error
	AssociateEDUWithDestination(ctx context.Context, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error

	GetNextTransactionPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) (gomatrixserverlib.TransactionID, []*gomatrixserverlib.HeaderedEvent, *shared.Receipt, error)
	GetNextTransactionEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, limit int) ([]*gomatrixserverlib.EDU, *shared.Receipt, error)

	CleanPDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error
	CleanEDUs(ctx context.Context, serverName gomatrixserverlib.ServerName, receipt *shared.Receipt) error

	GetPendingPDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error)
	GetPendingEDUCount(ctx context.Context, serverName gomatrixserverlib.ServerName) (int64, error)

	GetPendingPDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	GetPendingEDUServerNames(ctx context.Context) ([]gomatrixserverlib.ServerName, error)

	AddServerToBlacklist(serverName gomatrixserverlib.ServerName) error
	RemoveServerFromBlacklist(serverName gomatrixserverlib.ServerName) error
	IsServerBlacklisted(serverName gomatrixserverlib.ServerName) (bool, error)
}
