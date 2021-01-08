// Copyright 2021 The Matrix.org Foundation C.I.C.
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

// Package msc2946 'Spaces Summary' implements https://github.com/matrix-org/matrix-doc/pull/2946
package msc2946

import (
	"context"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/internal/hooks"
	"github.com/matrix-org/dendrite/internal/httputil"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// SpacesRequest is the request body to POST /_matrix/client/r0/rooms/{roomID}/spaces
type SpacesRequest struct {
	MaxRoomsPerSpace int    `json:"max_rooms_per_space"`
	Limit            int    `json:"limit"`
	Batch            string `json:"batch"`
}

// SpacesResponse is the response body to POST /_matrix/client/r0/rooms/{roomID}/spaces
type SpacesResponse struct {
	NextBatch string `json:"next_batch"`
	// Rooms are nodes on the space graph.
	Rooms []Room `json:"rooms"`
	// Events are edges on the space graph, exclusively m.space.child or m.room.parent events
	Events []gomatrixserverlib.ClientEvent `json:"events"`
}

// Room is a node on the space graph
type Room struct {
	gomatrixserverlib.PublicRoom
	NumRefs  int    `json:"num_refs"`
	RoomType string `json:"room_type"`
}

// Enable this MSC
func Enable(
	base *setup.BaseDendrite, rsAPI roomserver.RoomserverInternalAPI, userAPI userapi.UserInternalAPI,
) error {
	db, err := NewDatabase(&base.Cfg.MSCs.Database)
	if err != nil {
		return fmt.Errorf("Cannot enable MSC2946: %w", err)
	}
	hooks.Enable()
	hooks.Attach(hooks.KindNewEventPersisted, func(headeredEvent interface{}) {
		he := headeredEvent.(*gomatrixserverlib.HeaderedEvent)
		hookErr := db.StoreRelation(context.Background(), he)
		if hookErr != nil {
			util.GetLogger(context.Background()).WithError(hookErr).WithField("event_id", he.EventID()).Error(
				"failed to StoreRelation",
			)
		}
	})

	base.PublicClientAPIMux.Handle("/unstable/rooms/{roomID}/spaces",
		httputil.MakeAuthAPI("spaces", userAPI, spacesHandler(db, rsAPI)),
	).Methods(http.MethodPost, http.MethodOptions)
	return nil
}

func spacesHandler(db Database, rsAPI roomserver.RoomserverInternalAPI) func(*http.Request, *userapi.Device) util.JSONResponse {
	return func(req *http.Request, device *userapi.Device) util.JSONResponse {
		return util.JSONResponse{
			Code: 200,
			JSON: struct{}{},
		}
	}
}
