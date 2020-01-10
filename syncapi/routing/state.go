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

package routing

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	log "github.com/sirupsen/logrus"
)

type stateEventInStateResp struct {
	gomatrixserverlib.ClientEvent
	PrevContent   json.RawMessage `json:"prev_content,omitempty"`
	ReplacesState string          `json:"replaces_state,omitempty"`
}

// OnIncomingStateRequest is called when a client makes a /rooms/{roomID}/state
// request. It will fetch all the state events from the specified room and will
// append the necessary keys to them if applicable before returning them.
// Returns an error if something went wrong in the process.
// TODO: Check if the user is in the room. If not, check if the room's history
// is publicly visible. Current behaviour is returning an empty array if the
// user cannot see the room's history.
func OnIncomingStateRequest(req *http.Request, db storage.Database, roomID string) util.JSONResponse {
	// TODO(#287): Auth request and handle the case where the user has left (where
	// we should return the state at the poin they left)

	stateFilterPart := gomatrix.DefaultFilterPart()
	// TODO: stateFilterPart should not limit the number of state events (or only limits abusive number of events)

	stateEvents, err := db.GetStateEventsForRoom(req.Context(), roomID, &stateFilterPart)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	resp := []stateEventInStateResp{}
	// Fill the prev_content and replaces_state keys if necessary
	for _, event := range stateEvents {
		stateEvent := stateEventInStateResp{
			ClientEvent: gomatrixserverlib.ToClientEvent(event, gomatrixserverlib.FormatAll),
		}
		var prevEventRef types.PrevEventRef
		if len(event.Unsigned()) > 0 {
			if err := json.Unmarshal(event.Unsigned(), &prevEventRef); err != nil {
				return httputil.LogThenError(req, err)
			}
			// Fills the previous state event ID if the state event replaces another
			// state event
			if len(prevEventRef.ReplacesState) > 0 {
				stateEvent.ReplacesState = prevEventRef.ReplacesState
			}
			// Fill the previous event if the state event references a previous event
			if prevEventRef.PrevContent != nil {
				stateEvent.PrevContent = prevEventRef.PrevContent
			}
		}

		resp = append(resp, stateEvent)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp,
	}
}

// OnIncomingStateTypeRequest is called when a client makes a
// /rooms/{roomID}/state/{type}/{statekey} request. It will look in current
// state to see if there is an event with that type and state key, if there
// is then (by default) we return the content, otherwise a 404.
func OnIncomingStateTypeRequest(req *http.Request, db storage.Database, roomID string, evType, stateKey string) util.JSONResponse {
	// TODO(#287): Auth request and handle the case where the user has left (where
	// we should return the state at the poin they left)

	logger := util.GetLogger(req.Context())
	logger.WithFields(log.Fields{
		"roomID":   roomID,
		"evType":   evType,
		"stateKey": stateKey,
	}).Info("Fetching state")

	event, err := db.GetStateEvent(req.Context(), roomID, evType, stateKey)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	if event == nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("cannot find state"),
		}
	}

	stateEvent := stateEventInStateResp{
		ClientEvent: gomatrixserverlib.ToClientEvent(*event, gomatrixserverlib.FormatAll),
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: stateEvent.Content,
	}
}
