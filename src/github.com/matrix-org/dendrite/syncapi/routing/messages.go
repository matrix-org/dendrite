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
	// "encoding/json"
	"net/http"
	"strconv"

	// "github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type messageResp struct {
	Start string                          `json:"start"`
	End   string                          `json:"end"`
	Chunk []gomatrixserverlib.ClientEvent `json:"chunk"`
}

const defaultMessagesLimit = 10

func OnIncomingMessagesRequest(req *http.Request, db *storage.SyncServerDatabase, roomID string) util.JSONResponse {
	var from, to int
	var err error
	// Extract parameters from the request's URL.
	// Pagination tokens.
	from, err = strconv.Atoi(req.URL.Query().Get("from"))
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue("from could not be parsed into an integer: " + err.Error()),
		}
	}
	fromPos := types.StreamPosition(from)

	// Direction to return events from.
	dir := req.URL.Query().Get("dir")
	if dir != "b" && dir != "f" {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad or missing dir query parameter (should be either 'b' or 'f')"),
		}
	}
	backwardOrdering := (dir == "b")

	toStr := req.URL.Query().Get("to")
	var toPos types.StreamPosition
	if len(toStr) > 0 {
		to, err = strconv.Atoi(toStr)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("to could not be parsed into an integer: " + err.Error()),
			}
		}
		toPos = types.StreamPosition(to)
	} else {
		if backwardOrdering {
			toPos = types.StreamPosition(0)
		} else {
			toPos, err = db.SyncStreamPosition(req.Context())
			if err != nil {
				return jsonerror.InternalServerError()
			}
		}
	}

	// Maximum number of events to return; defaults to 10.
	limit := defaultMessagesLimit
	if len(req.URL.Query().Get("limit")) > 0 {
		limit, err = strconv.Atoi(req.URL.Query().Get("limit"))

		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: jsonerror.InvalidArgumentValue("limit could not be parsed into an integer: " + err.Error()),
			}
		}
	}
	// TODO: Implement filtering (#587)

	// Check the room ID's format.
	if _, _, err = gomatrixserverlib.SplitID('!', roomID); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument("Bad room ID: " + err.Error()),
		}
	}

	streamEvents, err := db.GetEventsInRange(
		req.Context(), fromPos, toPos, roomID, limit, backwardOrdering,
	)
	if err != nil {
		return jsonerror.InternalServerError()
	}

	// Check if we don't have enough events, i.e. len(sev) < limit and the events
	isSetLargeEnough := true
	if len(streamEvents) < limit {
		if backwardOrdering {
			if len(toStr) > 0 {
				// The condition in the SQL query is a strict "greater than" so
				// we need to check against to-1.
				isSetLargeEnough = (toPos-1 == streamEvents[0].StreamPosition)
			}
		} else {
			// We need all events from < streamPos < to
			isSetLargeEnough = (fromPos-1 == streamEvents[0].StreamPosition)
		}
	}
	// Check if earliest event is a backward extremity, i.e. if one of its
	// previous events is missing from the db.
	prevIDs := streamEvents[0].PrevEventIDs()
	prevs, err := db.Events(req.Context(), prevIDs)
	var eventInDB, isBackwardExtremity bool
	var id string
	for _, id = range prevIDs {
		eventInDB = false
		for _, ev := range prevs {
			if ev.EventID() == id {
				eventInDB = true
			}
		}
		if !eventInDB {
			isBackwardExtremity = true
			break
		}
	}

	if isBackwardExtremity && !isSetLargeEnough {
		log.WithFields(log.Fields{
			"limit":               limit,
			"nb_events":           len(streamEvents),
			"from":                fromPos.String(),
			"to":                  toPos.String(),
			"isBackwardExtremity": isBackwardExtremity,
			"isSetLargeEnough":    isSetLargeEnough,
		}).Info("Backfilling!")
		println("Backfilling!")
	}

	events := storage.StreamEventsToEvents(nil, streamEvents)
	clientEvents := gomatrixserverlib.ToClientEvents(events, gomatrixserverlib.FormatAll)

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: messageResp{
			Chunk: clientEvents,
			Start: streamEvents[0].StreamPosition.String(),
			End:   streamEvents[len(streamEvents)-1].StreamPosition.String(),
		},
	}
}
