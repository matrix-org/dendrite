// Copyright 2017 New Vector Ltd
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

package routing

import (
	"context"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

func MakeJoin(
	ctx context.Context,
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg config.Dendrite,
	query api.RoomserverQueryAPI,
	now time.Time,
	keys gomatrixserverlib.KeyRing,
	roomID, userID string,
) util.JSONResponse {
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &userID,
	}
	err := builder.SetContent(map[string]interface{}{"membership": "join"})
	if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	var queryRes api.QueryLatestEventsAndStateResponse
	event, err := common.BuildEvent(ctx, &builder, cfg, query, &queryRes)
	if err == common.ErrRoomNoExists {
		return util.JSONResponse{
			Code: 404,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	// check to see if this user can perform this operation
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = &queryRes.StateEvents[i]
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)
	if err = gomatrixserverlib.Allowed(*event, &provider); err != nil {
		return util.JSONResponse{
			Code: 403,
			JSON: jsonerror.Forbidden(err.Error()), // TODO: Is this error string comprehensible to the client?
		}
	}

	return util.JSONResponse{
		Code: 200,
		JSON: event,
	}
}

func SendJoin(
	ctx context.Context,
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg config.Dendrite,
	query api.RoomserverQueryAPI,
	now time.Time,
	keys gomatrixserverlib.KeyRing,
	roomID, eventID string,
) util.JSONResponse {

}
