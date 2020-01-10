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
	"net/url"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/postgres"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/typingserver/cache"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	common.PartitionStorer
	AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error)
	Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error)
	WriteEvent(ctx context.Context, ev *gomatrixserverlib.Event, addStateEvents []gomatrixserverlib.Event, addStateEventIDs, removeStateEventIDs []string, transactionID *api.TransactionID) (pduPosition int64, returnErr error)
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.Event, error)
	GetStateEventsForRoom(ctx context.Context, roomID string, stateFilterPart *gomatrix.FilterPart) (stateEvents []gomatrixserverlib.Event, err error)
	SyncPosition(ctx context.Context) (types.SyncPosition, error)
	IncrementalSync(ctx context.Context, device authtypes.Device, fromPos, toPos types.SyncPosition, numRecentEventsPerRoom int, wantFullState bool) (*types.Response, error)
	CompleteSync(ctx context.Context, userID string, numRecentEventsPerRoom int) (*types.Response, error)
	GetAccountDataInRange(ctx context.Context, userID string, oldPos, newPos int64, accountDataFilterPart *gomatrix.FilterPart) (map[string][]string, error)
	UpsertAccountData(ctx context.Context, userID, roomID, dataType string) (int64, error)
	AddInviteEvent(ctx context.Context, inviteEvent gomatrixserverlib.Event) (int64, error)
	RetireInviteEvent(ctx context.Context, inviteEventID string) error
	SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn)
	AddTypingUser(userID, roomID string, expireTime *time.Time) int64
	RemoveTypingUser(userID, roomID string) int64
}

// NewPublicRoomsServerDatabase opens a database connection.
func NewSyncServerDatasource(dataSourceName string) (Database, error) {
	uri, err := url.Parse(dataSourceName)
	if err != nil {
		return postgres.NewSyncServerDatasource(dataSourceName)
	}
	switch uri.Scheme {
	case "postgres":
		return postgres.NewSyncServerDatasource(dataSourceName)
	default:
		return postgres.NewSyncServerDatasource(dataSourceName)
	}
}
