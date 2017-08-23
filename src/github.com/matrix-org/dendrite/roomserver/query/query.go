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

package query

import (
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/state"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// RoomserverQueryAPIDatabase has the storage APIs needed to implement the query API.
type RoomserverQueryAPIDatabase interface {
	state.RoomStateDatabase
	// Look up the numeric ID for the room.
	// Returns 0 if the room doesn't exists.
	// Returns an error if there was a problem talking to the database.
	RoomNID(roomID string) (types.RoomNID, error)
	// Look up event references for the latest events in the room and the current state snapshot.
	// Returns the latest events, the current state and the maximum depth of the latest events plus 1.
	// Returns an error if there was a problem talking to the database.
	LatestEventIDs(roomNID types.RoomNID) ([]gomatrixserverlib.EventReference, types.StateSnapshotNID, int64, error)
	// Look up the numeric IDs for a list of events.
	// Returns an error if there was a problem talking to the database.
	EventNIDs(eventIDs []string) (map[string]types.EventNID, error)
	// Look up the join events for all members in a room as requested by a given
	// user. If the user is currently in the room, returns the room's current
	// members, if not returns an empty array (TODO: Fix it)
	// If the user requesting the list of members has never been in the room,
	// returns nil.
	// If there was an issue retrieving the events, returns an error.
	GetMembershipEvents(roomNID types.RoomNID, requestSenderUserID string) (events []types.Event, err error)
	// Look up the active invites targeting a user in a room and return the
	// numeric state key IDs for the user IDs who sent them.
	// Returns an error if there was a problem talking to the database.
	GetInvitesForUser(roomNID types.RoomNID, targetUserNID types.EventStateKeyNID) (senderUserNIDs []types.EventStateKeyNID, err error)
	// Look up the string event state keys for a list of numeric event state keys
	// Returns an error if there was a problem talking to the database.
	EventStateKeys([]types.EventStateKeyNID) (map[types.EventStateKeyNID]string, error)
}

// RoomserverQueryAPI is an implementation of api.RoomserverQueryAPI
type RoomserverQueryAPI struct {
	DB RoomserverQueryAPIDatabase
}

// QueryLatestEventsAndState implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryLatestEventsAndState(
	request *api.QueryLatestEventsAndStateRequest,
	response *api.QueryLatestEventsAndStateResponse,
) error {
	response.QueryLatestEventsAndStateRequest = *request
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true
	var currentStateSnapshotNID types.StateSnapshotNID
	response.LatestEvents, currentStateSnapshotNID, response.Depth, err = r.DB.LatestEventIDs(roomNID)
	if err != nil {
		return err
	}

	// Look up the currrent state for the requested tuples.
	stateEntries, err := state.LoadStateAtSnapshotForStringTuples(r.DB, currentStateSnapshotNID, request.StateToFetch)
	if err != nil {
		return err
	}

	stateEvents, err := r.loadStateEvents(stateEntries)
	if err != nil {
		return err
	}

	response.StateEvents = stateEvents
	return nil
}

// QueryStateAfterEvents implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryStateAfterEvents(
	request *api.QueryStateAfterEventsRequest,
	response *api.QueryStateAfterEventsResponse,
) error {
	response.QueryStateAfterEventsRequest = *request
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}
	if roomNID == 0 {
		return nil
	}
	response.RoomExists = true

	prevStates, err := r.DB.StateAtEventIDs(request.PrevEventIDs)
	if err != nil {
		switch err.(type) {
		case types.MissingEventError:
			return nil
		default:
			return err
		}
	}
	response.PrevEventsExist = true

	// Look up the currrent state for the requested tuples.
	stateEntries, err := state.LoadStateAfterEventsForStringTuples(r.DB, prevStates, request.StateToFetch)
	if err != nil {
		return err
	}

	stateEvents, err := r.loadStateEvents(stateEntries)
	if err != nil {
		return err
	}

	response.StateEvents = stateEvents
	return nil
}

// QueryEventsByID implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryEventsByID(
	request *api.QueryEventsByIDRequest,
	response *api.QueryEventsByIDResponse,
) error {
	response.QueryEventsByIDRequest = *request

	eventNIDMap, err := r.DB.EventNIDs(request.EventIDs)
	if err != nil {
		return err
	}

	var eventNIDs []types.EventNID
	for _, nid := range eventNIDMap {
		eventNIDs = append(eventNIDs, nid)
	}

	events, err := r.loadEvents(eventNIDs)
	if err != nil {
		return err
	}

	response.Events = events
	return nil
}

func (r *RoomserverQueryAPI) loadStateEvents(stateEntries []types.StateEntry) ([]gomatrixserverlib.Event, error) {
	eventNIDs := make([]types.EventNID, len(stateEntries))
	for i := range stateEntries {
		eventNIDs[i] = stateEntries[i].EventNID
	}
	return r.loadEvents(eventNIDs)
}

func (r *RoomserverQueryAPI) loadEvents(eventNIDs []types.EventNID) ([]gomatrixserverlib.Event, error) {
	stateEvents, err := r.DB.Events(eventNIDs)
	if err != nil {
		return nil, err
	}

	result := make([]gomatrixserverlib.Event, len(stateEvents))
	for i := range stateEvents {
		result[i] = stateEvents[i].Event
	}
	return result, nil
}

// QueryMembershipsForRoom implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryMembershipsForRoom(
	request *api.QueryMembershipsForRoomRequest,
	response *api.QueryMembershipsForRoomResponse,
) error {
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}

	events, err := r.DB.GetMembershipEvents(roomNID, request.Sender)
	if err != nil {
		return nil
	}

	if events == nil {
		response.HasBeenInRoom = false
		response.JoinEvents = nil
		return nil
	}

	response.HasBeenInRoom = true
	response.JoinEvents = []gomatrixserverlib.ClientEvent{}
	for _, event := range events {
		clientEvent := gomatrixserverlib.ToClientEvent(event.Event, gomatrixserverlib.FormatAll)
		response.JoinEvents = append(response.JoinEvents, clientEvent)
	}

	return nil
}

// QueryInvitesForUser implements api.RoomserverQueryAPI
func (r *RoomserverQueryAPI) QueryInvitesForUser(
	request *api.QueryInvitesForUserRequest,
	response *api.QueryInvitesForUserResponse,
) error {
	roomNID, err := r.DB.RoomNID(request.RoomID)
	if err != nil {
		return err
	}

	targetUserNIDs, err := r.DB.EventStateKeyNIDs([]string{request.TargetUserID})
	if err != nil {
		return err
	}
	targetUserNID := targetUserNIDs[request.TargetUserID]

	senderUserNIDs, err := r.DB.GetInvitesForUser(roomNID, targetUserNID)
	if err != nil {
		return err
	}

	senderUserIDs, err := r.DB.EventStateKeys(senderUserNIDs)
	if err != nil {
		return err
	}

	for _, senderUserID := range senderUserIDs {
		response.InviteSenderUserIDs = append(response.InviteSenderUserIDs, senderUserID)
	}

	return nil
}

// SetupHTTP adds the RoomserverQueryAPI handlers to the http.ServeMux.
func (r *RoomserverQueryAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(
		api.RoomserverQueryLatestEventsAndStatePath,
		common.MakeAPI("queryLatestEventsAndState", func(req *http.Request) util.JSONResponse {
			var request api.QueryLatestEventsAndStateRequest
			var response api.QueryLatestEventsAndStateResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryLatestEventsAndState(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryStateAfterEventsPath,
		common.MakeAPI("queryStateAfterEvents", func(req *http.Request) util.JSONResponse {
			var request api.QueryStateAfterEventsRequest
			var response api.QueryStateAfterEventsResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryStateAfterEvents(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryEventsByIDPath,
		common.MakeAPI("queryEventsByID", func(req *http.Request) util.JSONResponse {
			var request api.QueryEventsByIDRequest
			var response api.QueryEventsByIDResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryEventsByID(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryMembershipsForRoomPath,
		common.MakeAPI("queryMembershipsForRoom", func(req *http.Request) util.JSONResponse {
			var request api.QueryMembershipsForRoomRequest
			var response api.QueryMembershipsForRoomResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryMembershipsForRoom(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
	servMux.Handle(
		api.RoomserverQueryInvitesForUserPath,
		common.MakeAPI("queryInvitesForUser", func(req *http.Request) util.JSONResponse {
			var request api.QueryInvitesForUserRequest
			var response api.QueryInvitesForUserResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.ErrorResponse(err)
			}
			if err := r.QueryInvitesForUser(&request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: 200, JSON: &response}
		}),
	)
}
