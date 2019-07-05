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
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type getMissingEventRequest struct {
	EarliestEvents []string `json:"earliest_events"`
	LatestEvents   []string `json:"latest_events"`
	Limit          int      `json:"limit"`
	MinDepth       int64    `json:"min_depth"`
}

// GetMissingEvents returns missing events between earliest_events & latest_events.
// Events are fetched from room DAG starting from latest_events until we reach earliest_events or the limit.
func GetMissingEvents(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	query api.RoomserverQueryAPI,
	roomID string,
) util.JSONResponse {
	var gme getMissingEventRequest
	if err := json.Unmarshal(request.Content(), &gme); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("The request body could not be decoded into valid JSON. " + err.Error()),
		}
	}

	var eventsResponse api.QueryMissingEventsResponse
	if err := query.QueryMissingEvents(
		httpReq.Context(), &api.QueryMissingEventsRequest{
			EarliestEvents: gme.EarliestEvents,
			LatestEvents:   gme.LatestEvents,
			Limit:          gme.Limit,
			ServerName:     request.Origin(),
		},
		&eventsResponse,
	); err != nil {
		return httputil.LogThenError(httpReq, err)
	}

	eventsResponse.Events = filterEvents(eventsResponse.Events, gme.MinDepth, roomID)
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: eventsResponse,
	}
}

// filterEvents returns only those events with matching roomID and having depth greater than minDepth
func filterEvents(
	events []gomatrixserverlib.Event, minDepth int64, roomID string,
) []gomatrixserverlib.Event {
	ref := events[:0]
	for _, ev := range events {
		if ev.Depth() >= minDepth && ev.RoomID() == roomID {
			ref = append(ref, ev)
		}
	}
	return ref
}
