// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package perform

import (
	"context"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/storage"
)

type Admin struct {
	DB     storage.Database
	Leaver *Leaver
}

// PerformEvacuateRoom will remove all local users from the given room.
func (r *Admin) PerformAdminEvacuateRoom(
	ctx context.Context,
	req *api.PerformAdminEvacuateRoomRequest,
	res *api.PerformAdminEvacuateRoomResponse,
) {
	roomInfo, err := r.DB.RoomInfo(ctx, req.RoomID)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.RoomInfo: %s", err),
		}
		return
	}
	if roomInfo.IsStub {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  "room is stub",
		}
		return
	}

	memberNIDs, err := r.DB.GetMembershipEventNIDsForRoom(ctx, roomInfo.RoomNID, true, true)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.GetMembershipEventNIDsForRoom: %s", err),
		}
		return
	}

	memberEvents, err := r.DB.Events(ctx, memberNIDs)
	if err != nil {
		res.Error = &api.PerformError{
			Code: api.PerformErrorBadRequest,
			Msg:  fmt.Sprintf("r.DB.Events: %s", err),
		}
		return
	}

	for _, memberEvent := range memberEvents {
		if memberEvent.StateKey() == nil {
			continue
		}
		userID := *memberEvent.StateKey()
		leaveReq := &api.PerformLeaveRequest{
			RoomID: req.RoomID,
			UserID: userID,
		}
		leaveRes := &api.PerformLeaveResponse{}
		if _, err = r.Leaver.PerformLeave(ctx, leaveReq, leaveRes); err != nil {
			res.Error = &api.PerformError{
				Code: api.PerformErrorBadRequest,
				Msg:  fmt.Sprintf("r.Leaver.PerformLeave (%s): %s", userID, err),
			}
			return
		}
	}
}
