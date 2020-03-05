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
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/dendrite/typingserver/cache"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	common.PartitionStorer
	AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error)
	Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.Event, error)
	WriteEvent(context.Context, *gomatrixserverlib.Event, []gomatrixserverlib.Event, []string, []string, *api.TransactionID, bool) (types.StreamPosition, error)
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.Event, error)
	GetStateEventsForRoom(ctx context.Context, roomID string, stateFilterPart *gomatrixserverlib.StateFilter) (stateEvents []gomatrixserverlib.Event, err error)
	SyncPosition(ctx context.Context) (types.PaginationToken, error)
	IncrementalSync(ctx context.Context, device authtypes.Device, fromPos, toPos types.PaginationToken, numRecentEventsPerRoom int, wantFullState bool) (*types.Response, error)
	CompleteSync(ctx context.Context, userID string, numRecentEventsPerRoom int) (*types.Response, error)
	GetAccountDataInRange(ctx context.Context, userID string, oldPos, newPos types.StreamPosition, accountDataFilterPart *gomatrixserverlib.EventFilter) (map[string][]string, error)
	UpsertAccountData(ctx context.Context, userID, roomID, dataType string) (types.StreamPosition, error)
	AddInviteEvent(ctx context.Context, inviteEvent gomatrixserverlib.Event) (types.StreamPosition, error)
	RetireInviteEvent(ctx context.Context, inviteEventID string) error
	SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn)
	AddTypingUser(userID, roomID string, expireTime *time.Time) types.StreamPosition
	RemoveTypingUser(userID, roomID string) types.StreamPosition
	GetEventsInRange(ctx context.Context, from, to *types.PaginationToken, roomID string, limit int, backwardOrdering bool) (events []types.StreamEvent, err error)
	EventPositionInTopology(ctx context.Context, eventID string) (types.StreamPosition, error)
	EventsAtTopologicalPosition(ctx context.Context, roomID string, pos types.StreamPosition) ([]types.StreamEvent, error)
	BackwardExtremitiesForRoom(ctx context.Context, roomID string) (backwardExtremities []string, err error)
	MaxTopologicalPosition(ctx context.Context, roomID string) (types.StreamPosition, error)
	StreamEventsToEvents(device *authtypes.Device, in []types.StreamEvent) []gomatrixserverlib.Event
	SyncStreamPosition(ctx context.Context) (types.StreamPosition, error)
}
