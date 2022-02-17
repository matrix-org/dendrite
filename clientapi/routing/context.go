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

package routing

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type ContextRespsonse struct {
	End          string                          `json:"end,omitempty"`
	Event        gomatrixserverlib.ClientEvent   `json:"event"`
	EventsAfter  []gomatrixserverlib.ClientEvent `json:"events_after,omitempty"`
	EventsBefore []gomatrixserverlib.ClientEvent `json:"events_before,omitempty"`
	Start        string                          `json:"start,omitempty"`
	State        []gomatrixserverlib.ClientEvent `json:"state,omitempty"`
}

func Context(
	req *http.Request, device *userapi.Device,
	rsAPI roomserver.RoomserverInternalAPI,
	userAPI userapi.UserInternalAPI,
	roomID, eventID string,
) util.JSONResponse {
	limit, filter, err := parseContextParams(req)
	if err != nil {
		errMsg := ""
		switch err.(type) {
		case *json.InvalidUnmarshalError:
			errMsg = "unable to parse filter"
		case *strconv.NumError:
			errMsg = "unable to parse limit"
		}
		return util.JSONResponse{
			Code:    http.StatusBadRequest,
			JSON:    jsonerror.InvalidParam(errMsg),
			Headers: nil,
		}
	}
	ctx := req.Context()

	membershipRes := roomserver.QueryMembershipForUserResponse{}
	membershipReq := roomserver.QueryMembershipForUserRequest{UserID: device.UserID, RoomID: roomID}
	if err := rsAPI.QueryMembershipForUser(ctx, &membershipReq, &membershipRes); err != nil {
		return jsonerror.InternalServerError()
	}

	_ = filter

	avatarTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.avatar", StateKey: ""}
	nameTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.name", StateKey: ""}
	canonicalTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomCanonicalAlias, StateKey: ""}
	topicTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.topic", StateKey: ""}
	guestTuple := gomatrixserverlib.StateKeyTuple{EventType: "m.room.guest_access", StateKey: ""}
	visibilityTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomHistoryVisibility, StateKey: ""}
	joinRuleTuple := gomatrixserverlib.StateKeyTuple{EventType: gomatrixserverlib.MRoomJoinRules, StateKey: ""}

	currentState := &roomserver.QueryCurrentStateResponse{}
	if err := rsAPI.QueryCurrentState(ctx, &roomserver.QueryCurrentStateRequest{
		RoomID: roomID,
		StateTuples: []gomatrixserverlib.StateKeyTuple{
			avatarTuple, nameTuple, canonicalTuple, topicTuple, guestTuple, visibilityTuple, joinRuleTuple,
		},
	}, currentState); err != nil {
		logrus.WithField("roomID", roomID).WithError(err).Error("unable to fetch current state")
		return jsonerror.InternalServerError()
	}

	state := []gomatrixserverlib.ClientEvent{}
	for tuple, event := range currentState.StateEvents {
		// check that the user is allowed to view the context
		if tuple == visibilityTuple {
			hisVis, err := event.HistoryVisibility()
			if err != nil {
				return jsonerror.InternalServerError()
			}
			allowed := hisVis != "world_readable" && membershipRes.Membership == "join"
			if !allowed {
				return util.JSONResponse{
					Code: http.StatusForbidden,
				}
			}
		}
		state = append(state, gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatAll))
	}

	requestEvent := &roomserver.QueryEventsByIDResponse{}
	if err := rsAPI.QueryEventsByID(ctx, &roomserver.QueryEventsByIDRequest{
		EventIDs: []string{eventID},
	}, requestEvent); err != nil {
		return jsonerror.InternalServerError()
	}
	if requestEvent.Events == nil || len(requestEvent.Events) == 0 {
		logrus.WithField("eventID", eventID).Error("unable to find requested event")
		return jsonerror.InternalServerError()
	}
	// this should be safe now
	event := requestEvent.Events[0]

	eventsBefore, err := queryEventsBefore(rsAPI, ctx, event.PrevEventIDs(), limit)
	if err != nil && err != sql.ErrNoRows {
		logrus.WithError(err).Error("unable to fetch before events")
		return jsonerror.InternalServerError()
	}

	eventsAfter, err := queryEventsAfter(rsAPI, ctx, event.EventID(), limit)
	if err != nil {
		logrus.WithError(err).Error("unable to fetch after events")
		return jsonerror.InternalServerError()
	}

	response := ContextRespsonse{
		End:          "end",
		Event:        gomatrixserverlib.HeaderedToClientEvent(event, gomatrixserverlib.FormatAll),
		EventsAfter:  eventsAfter,
		EventsBefore: eventsBefore,
		Start:        "start",
		State:        state,
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

// queryEventsAfter retrieves events that happened after a list of events.
// The function returns once the limit is reached or no new events can be found.
// TODO: inefficient
func queryEventsAfter(
	rsAPI roomserver.RoomserverInternalAPI,
	ctx context.Context,
	eventID string,
	limit int,
) ([]gomatrixserverlib.ClientEvent, error) {
	result := []gomatrixserverlib.ClientEvent{}
	for {
		res := &roomserver.QueryEventsAfterEventIDesponse{}
		if err := rsAPI.QueryEventsAfter(ctx, &roomserver.QueryEventsAfterEventIDRequest{EventIDs: eventID}, res); err != nil {
			if err == sql.ErrNoRows {
				return result, nil
			}
			return nil, err
		}
		if len(res.Events) > 0 {
			for _, ev := range res.Events {
				result = append(result, *ev)
				eventID = ev.EventID
			}
		}
	}
}

// queryEventsBefore retrieves events that happened before a list of events.
// The function returns once the limit is reached or no new prevEvents can be found.
// TODO: inefficient
func queryEventsBefore(
	rsAPI roomserver.RoomserverInternalAPI,
	ctx context.Context,
	prevEventIDs []string,
	limit int,
) ([]gomatrixserverlib.ClientEvent, error) {
	// query prev events
	eventIDs := prevEventIDs
	result := []*gomatrixserverlib.HeaderedEvent{}
	for len(eventIDs) > 0 {
		prevEvents := &roomserver.QueryEventsByIDResponse{}
		if err := rsAPI.QueryEventsByID(ctx, &roomserver.QueryEventsByIDRequest{
			EventIDs: eventIDs,
		}, prevEvents); err != nil {
			return gomatrixserverlib.HeaderedToClientEvents(result, gomatrixserverlib.FormatAll), err
		}
		// we didn't receive any events, return
		if len(prevEvents.Events) == 0 {
			return gomatrixserverlib.HeaderedToClientEvents(result, gomatrixserverlib.FormatAll), nil
		}
		// clear eventIDs to search for
		eventIDs = []string{}
		// append found events to result
		for _, ev := range prevEvents.Events {
			result = append(result, ev)
			if len(result) >= limit {
				return gomatrixserverlib.HeaderedToClientEvents(result, gomatrixserverlib.FormatAll), nil
			}
			// add prev to new eventIDs
			eventIDs = append(eventIDs, ev.PrevEventIDs()...)
		}
	}

	return gomatrixserverlib.HeaderedToClientEvents(result, gomatrixserverlib.FormatAll), nil
}

func parseContextParams(req *http.Request) (limit int, filter *gomatrixserverlib.RoomEventFilter, err error) {
	l := req.URL.Query().Get("limit")
	f := req.URL.Query().Get("filter")
	limit = 10
	if l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil {
			return 0, filter, err
		}
		// not in the spec, but feels like a good idea to have an upper bound limit
		if limit > 100 {
			limit = 100
		}
	}
	if f != "" {
		if err := json.Unmarshal([]byte(f), &filter); err != nil {
			return 0, filter, err
		}
	}
	return limit, filter, nil
}
