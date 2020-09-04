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

package internal

import (
	"context"

	"github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/currentstateserver/storage"
	"github.com/matrix-org/gomatrixserverlib"
)

type CurrentStateInternalAPI struct {
	DB storage.Database
}

func (a *CurrentStateInternalAPI) QueryRoomsForUser(ctx context.Context, req *api.QueryRoomsForUserRequest, res *api.QueryRoomsForUserResponse) error {
	roomIDs, err := a.DB.GetRoomsByMembership(ctx, req.UserID, req.WantMembership)
	if err != nil {
		return err
	}
	res.RoomIDs = roomIDs
	return nil
}

func (a *CurrentStateInternalAPI) QueryBulkStateContent(ctx context.Context, req *api.QueryBulkStateContentRequest, res *api.QueryBulkStateContentResponse) error {
	events, err := a.DB.GetBulkStateContent(ctx, req.RoomIDs, req.StateTuples, req.AllowWildcards)
	if err != nil {
		return err
	}
	res.Rooms = make(map[string]map[gomatrixserverlib.StateKeyTuple]string)
	for _, ev := range events {
		if res.Rooms[ev.RoomID] == nil {
			res.Rooms[ev.RoomID] = make(map[gomatrixserverlib.StateKeyTuple]string)
		}
		room := res.Rooms[ev.RoomID]
		room[gomatrixserverlib.StateKeyTuple{
			EventType: ev.EventType,
			StateKey:  ev.StateKey,
		}] = ev.ContentValue
		res.Rooms[ev.RoomID] = room
	}
	return nil
}
