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
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/caching"
	roomserver "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type ContextRespsonse struct {
	End          string                          `json:"end"`
	Event        gomatrixserverlib.ClientEvent   `json:"event"`
	EventsAfter  []gomatrixserverlib.ClientEvent `json:"events_after,omitempty"`
	EventsBefore []gomatrixserverlib.ClientEvent `json:"events_before,omitempty"`
	Start        string                          `json:"start"`
	State        []gomatrixserverlib.ClientEvent `json:"state"`
}

func Context(
	req *http.Request, device *userapi.Device,
	rsAPI roomserver.RoomserverInternalAPI,
	syncDB storage.Database,
	roomID, eventID string,
	lazyLoadCache *caching.LazyLoadCache,
) util.JSONResponse {
	filter, err := parseRoomEventFilter(req)
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
	if filter.Rooms != nil {
		*filter.Rooms = append(*filter.Rooms, roomID)
	}

	ctx := req.Context()
	membershipRes := roomserver.QueryMembershipForUserResponse{}
	membershipReq := roomserver.QueryMembershipForUserRequest{UserID: device.UserID, RoomID: roomID}
	if err = rsAPI.QueryMembershipForUser(ctx, &membershipReq, &membershipRes); err != nil {
		logrus.WithError(err).Error("unable to query membership")
		return jsonerror.InternalServerError()
	}

	stateFilter := gomatrixserverlib.StateFilter{
		Limit:                   100,
		NotSenders:              filter.NotSenders,
		NotTypes:                filter.NotTypes,
		Senders:                 filter.Senders,
		Types:                   filter.Types,
		LazyLoadMembers:         filter.LazyLoadMembers,
		IncludeRedundantMembers: filter.IncludeRedundantMembers,
		NotRooms:                filter.NotRooms,
		Rooms:                   filter.Rooms,
		ContainsURL:             filter.ContainsURL,
	}

	// TODO: Get the actual state at the last event returned by SelectContextAfterEvent
	state, _ := syncDB.CurrentState(ctx, roomID, &stateFilter, nil)
	// verify the user is allowed to see the context for this room/event
	for _, x := range state {
		var hisVis string
		hisVis, err = x.HistoryVisibility()
		if err != nil {
			continue
		}
		allowed := hisVis == gomatrixserverlib.WorldReadable || membershipRes.Membership == gomatrixserverlib.Join
		if !allowed {
			return util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonerror.Forbidden("User is not allowed to query context"),
			}
		}
	}

	id, requestedEvent, err := syncDB.SelectContextEvent(ctx, roomID, eventID)
	if err != nil {
		if err == sql.ErrNoRows {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound(fmt.Sprintf("Event %s not found", eventID)),
			}
		}
		logrus.WithError(err).WithField("eventID", eventID).Error("unable to find requested event")
		return jsonerror.InternalServerError()
	}

	eventsBefore, err := syncDB.SelectContextBeforeEvent(ctx, id, roomID, filter)
	if err != nil && err != sql.ErrNoRows {
		logrus.WithError(err).Error("unable to fetch before events")
		return jsonerror.InternalServerError()
	}

	_, eventsAfter, err := syncDB.SelectContextAfterEvent(ctx, id, roomID, filter)
	if err != nil && err != sql.ErrNoRows {
		logrus.WithError(err).Error("unable to fetch after events")
		return jsonerror.InternalServerError()
	}

	eventsBeforeClient := gomatrixserverlib.HeaderedToClientEvents(eventsBefore, gomatrixserverlib.FormatAll)
	eventsAfterClient := gomatrixserverlib.HeaderedToClientEvents(eventsAfter, gomatrixserverlib.FormatAll)
	newState := applyLazyLoadMembers(device, filter, eventsAfterClient, eventsBeforeClient, state, lazyLoadCache)

	response := ContextRespsonse{
		Event:        gomatrixserverlib.HeaderedToClientEvent(&requestedEvent, gomatrixserverlib.FormatAll),
		EventsAfter:  eventsAfterClient,
		EventsBefore: eventsBeforeClient,
		State:        gomatrixserverlib.HeaderedToClientEvents(newState, gomatrixserverlib.FormatAll),
	}

	if len(response.State) > filter.Limit {
		response.State = response.State[len(response.State)-filter.Limit:]
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: response,
	}
}

func applyLazyLoadMembers(
	device *userapi.Device,
	filter *gomatrixserverlib.RoomEventFilter,
	eventsAfter, eventsBefore []gomatrixserverlib.ClientEvent,
	state []*gomatrixserverlib.HeaderedEvent,
	lazyLoadCache *caching.LazyLoadCache,
) []*gomatrixserverlib.HeaderedEvent {
	if filter == nil || !filter.LazyLoadMembers {
		return state
	}
	allEvents := append(eventsBefore, eventsAfter...)
	x := make(map[string]struct{})
	// get members who actually send an event
	for _, e := range allEvents {
		// Don't add membership events the client should already know about
		if _, cached := lazyLoadCache.IsLazyLoadedUserCached(device, e.RoomID, e.Sender); cached {
			continue
		}
		x[e.Sender] = struct{}{}
	}

	newState := []*gomatrixserverlib.HeaderedEvent{}
	membershipEvents := []*gomatrixserverlib.HeaderedEvent{}
	for _, event := range state {
		if event.Type() != gomatrixserverlib.MRoomMember {
			newState = append(newState, event)
		} else {
			// did the user send an event?
			if _, ok := x[event.Sender()]; ok {
				membershipEvents = append(membershipEvents, event)
				lazyLoadCache.StoreLazyLoadedUser(device, event.RoomID(), event.Sender(), event.EventID())
			}
		}
	}
	// Add the membershipEvents to the end of the list, to make Sytest happy
	return append(newState, membershipEvents...)
}

func parseRoomEventFilter(req *http.Request) (*gomatrixserverlib.RoomEventFilter, error) {
	// Default room filter
	filter := &gomatrixserverlib.RoomEventFilter{Limit: 10}

	l := req.URL.Query().Get("limit")
	f := req.URL.Query().Get("filter")
	if l != "" {
		limit, err := strconv.Atoi(l)
		if err != nil {
			return nil, err
		}
		// NOTSPEC: feels like a good idea to have an upper bound limit
		if limit > 100 {
			limit = 100
		}
		filter.Limit = limit
	}
	if f != "" {
		if err := json.Unmarshal([]byte(f), &filter); err != nil {
			return nil, err
		}
	}

	return filter, nil
}
