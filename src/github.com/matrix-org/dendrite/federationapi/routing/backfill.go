// Copyright 2018 New Vector Ltd
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
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/federationapi/types"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// Backfill implements the /backfill federation endpoint.
// https://matrix.org/docs/spec/server_server/unstable.html#get-matrix-federation-v1-backfill-roomid
func Backfill(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	query api.RoomserverQueryAPI,
	roomID string,
	cfg config.Dendrite,
) util.JSONResponse {
	var res api.QueryBackfillResponse
	var eIDs []string
	var limit string
	var exists bool
	var err error

	// Check the room ID's format.
	if _, _, err = gomatrixserverlib.SplitID('!', roomID); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad room ID: " + err.Error()),
		}
	}

	// Check if all of the required parameters are there.
	eIDs, exists = httpReq.URL.Query()["v"]
	if !exists {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("v is missing"),
		}
	}
	limit = httpReq.URL.Query().Get("limit")
	if len(limit) == 0 {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("limit is missing"),
		}
	}

	// Populate the request.
	req := api.QueryBackfillRequest{
		EarliestEventsIDs: eIDs,
		ServerName:        request.Origin(),
	}
	if req.Limit, err = strconv.Atoi(limit); err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	// Query the roomserver.
	if err = query.QueryBackfill(httpReq.Context(), &req, &res); err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	// Filter any event that's not from the requested room out.
	evs := make([]gomatrixserverlib.Event, 0)

	var ev gomatrixserverlib.Event
	for _, ev = range res.Events {
		if ev.RoomID() == roomID {
			evs = append(evs, ev)
		}
	}

	txn := types.NewTransaction()
	txn.Origin = cfg.Matrix.ServerName
	txn.PDUs = evs

	// Send the events to the client.
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: txn,
	}
}
