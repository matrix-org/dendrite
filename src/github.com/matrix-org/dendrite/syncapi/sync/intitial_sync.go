// Copyright 2017 Vector Creations Ltd
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

package sync

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// GetInitialSync gets called on an /initialSync request.
func GetInitialSync(
	req *http.Request, device *authtypes.Device,
	db *storage.SyncServerDatabase,
) util.JSONResponse {
	// TODO: Support limit
	syncResponse, err := db.CompleteSync(
		req.Context(), device.UserID, DefaultTimelineLimit,
	)

	if err != nil {
		return httputil.LogThenError(req, err)
	}

	intialSyncResponse := syncResponseToIntialSync(syncResponse)

	return util.JSONResponse{
		Code: 200,
		JSON: intialSyncResponse,
	}
}

func syncResponseToIntialSync(syncResponse *types.Response) initialSyncResponse {
	var rooms []initialSyncRoomResponse

	for roomID, room := range syncResponse.Rooms.Join {
		rooms = append(rooms, initialSyncRoomResponse{
			RoomID:     roomID,
			Membership: "join",

			Messages: initialSyncChunk{
				Chunk: room.Timeline.Events,    // TODO: Correctly format events
				End:   room.Timeline.PrevBatch, // TODO: Start?
			},

			State: initialSyncChunk{
				Chunk: room.State.Events, // TODO: Add state from timeline
			},
		})
	}

	// TODO: Invites, leaves, account data, presence, etc.

	return initialSyncResponse{
		Rooms: rooms,
		End:   syncResponse.NextBatch,
	}
}

type initialSyncResponse struct {
	End   string                    `json:"end"`
	Rooms []initialSyncRoomResponse `json:"rooms"`
}

type initialSyncRoomResponse struct {
	Membership string           `json:"membership"`
	RoomID     string           `json:"room_id"`
	Messages   initialSyncChunk `json:"messages,omitempty"`
	State      initialSyncChunk `json:"state,omitempty"`
}

type initialSyncChunk struct {
	Start string                          `json:"start"`
	End   string                          `json:"end"`
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}
