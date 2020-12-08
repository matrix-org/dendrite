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

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type FederationSenderQueuePDUs interface {
	InsertQueuePDU(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid types.ContentNID) error
	DeleteQueuePDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []types.ContentNID) error
	SelectQueuePDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID types.ContentNID) (types.ContentNID, error)
	SelectQueuePDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (types.ContentNID, error)
	SelectQueuePDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]types.ContentNID, error)
	SelectQueuePDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderQueueEDUs interface {
	InsertQueueEDU(ctx context.Context, txn *sql.Tx, eduType string, serverName gomatrixserverlib.ServerName, nid types.ContentNID) error
	DeleteQueueEDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []types.ContentNID) error
	SelectQueueEDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]types.ContentNID, error)
	SelectQueueEDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID types.ContentNID) (types.ContentNID, error)
	SelectQueueEDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (types.ContentNID, error)
	SelectQueueEDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderQueueJSON interface {
	InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (types.ContentNID, error)
	DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []types.ContentNID) error
	SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []types.ContentNID) (map[types.ContentNID][]byte, error)
}

type FederationSenderJoinedHosts interface {
	InsertJoinedHosts(ctx context.Context, txn *sql.Tx, roomID, eventID string, serverName gomatrixserverlib.ServerName) error
	DeleteJoinedHosts(ctx context.Context, txn *sql.Tx, eventIDs []string) error
	DeleteJoinedHostsForRoom(ctx context.Context, txn *sql.Tx, roomID string) error
	SelectJoinedHostsWithTx(ctx context.Context, txn *sql.Tx, roomID string) ([]types.JoinedHost, error)
	SelectJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	SelectAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
	SelectJoinedHostsForRooms(ctx context.Context, roomIDs []string) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderRooms interface {
	InsertRoom(ctx context.Context, txn *sql.Tx, roomID string) error
	SelectRoomForUpdate(ctx context.Context, txn *sql.Tx, roomID string) (string, error)
	UpdateRoom(ctx context.Context, txn *sql.Tx, roomID, lastEventID string) error
}

type FederationSenderBlacklist interface {
	InsertBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) error
	SelectBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (bool, error)
	DeleteBlacklist(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) error
}
