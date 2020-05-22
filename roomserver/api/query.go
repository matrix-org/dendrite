// Copyright 2017 Vector Creations Ltd
// Copyright 2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package api

import (
	"context"

	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	opentracing "github.com/opentracing/opentracing-go"
)

// QueryLatestEventsAndStateRequest is a request to QueryLatestEventsAndState
type QueryLatestEventsAndStateRequest struct {
	// The room ID to query the latest events for.
	RoomID string `json:"room_id"`
	// The state key tuples to fetch from the room current state.
	// If this list is empty or nil then *ALL* current state events are returned.
	StateToFetch []gomatrixserverlib.StateKeyTuple `json:"state_to_fetch"`
}

// QueryLatestEventsAndStateResponse is a response to QueryLatestEventsAndState
// This is used when sending events to set the prev_events, auth_events and depth.
// It is also used to tell whether the event is allowed by the event auth rules.
type QueryLatestEventsAndStateResponse struct {
	// Copy of the request for debugging.
	QueryLatestEventsAndStateRequest
	// Does the room exist?
	// If the room doesn't exist this will be false and LatestEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// The room version of the room.
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
	// The latest events in the room.
	// These are used to set the prev_events when sending an event.
	LatestEvents []gomatrixserverlib.EventReference `json:"latest_events"`
	// The state events requested.
	// This list will be in an arbitrary order.
	// These are used to set the auth_events when sending an event.
	// These are used to check whether the event is allowed.
	StateEvents []gomatrixserverlib.HeaderedEvent `json:"state_events"`
	// The depth of the latest events.
	// This is one greater than the maximum depth of the latest events.
	// This is used to set the depth when sending an event.
	Depth int64 `json:"depth"`
}

// QueryStateAfterEventsRequest is a request to QueryStateAfterEvents
type QueryStateAfterEventsRequest struct {
	// The room ID to query the state in.
	RoomID string `json:"room_id"`
	// The list of previous events to return the events after.
	PrevEventIDs []string `json:"prev_event_ids"`
	// The state key tuples to fetch from the state
	StateToFetch []gomatrixserverlib.StateKeyTuple `json:"state_to_fetch"`
}

// QueryStateAfterEventsResponse is a response to QueryStateAfterEvents
type QueryStateAfterEventsResponse struct {
	// Copy of the request for debugging.
	QueryStateAfterEventsRequest
	// Does the room exist on this roomserver?
	// If the room doesn't exist this will be false and StateEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// The room version of the room.
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
	// Do all the previous events exist on this roomserver?
	// If some of previous events do not exist this will be false and StateEvents will be empty.
	PrevEventsExist bool `json:"prev_events_exist"`
	// The state events requested.
	// This list will be in an arbitrary order.
	StateEvents []gomatrixserverlib.HeaderedEvent `json:"state_events"`
}

// QueryEventsByIDRequest is a request to QueryEventsByID
type QueryEventsByIDRequest struct {
	// The event IDs to look up.
	EventIDs []string `json:"event_ids"`
}

// QueryEventsByIDResponse is a response to QueryEventsByID
type QueryEventsByIDResponse struct {
	// Copy of the request for debugging.
	QueryEventsByIDRequest
	// A list of events with the requested IDs.
	// If the roomserver does not have a copy of a requested event
	// then it will omit that event from the list.
	// If the roomserver thinks it has a copy of the event, but
	// fails to read it from the database then it will fail
	// the entire request.
	// This list will be in an arbitrary order.
	Events []gomatrixserverlib.HeaderedEvent `json:"events"`
}

// QueryMembershipForUserRequest is a request to QueryMembership
type QueryMembershipForUserRequest struct {
	// ID of the room to fetch membership from
	RoomID string `json:"room_id"`
	// ID of the user for whom membership is requested
	UserID string `json:"user_id"`
}

// QueryMembershipForUserResponse is a response to QueryMembership
type QueryMembershipForUserResponse struct {
	// The EventID of the latest "m.room.member" event for the sender,
	// if HasBeenInRoom is true.
	EventID string `json:"event_id"`
	// True if the user has been in room before and has either stayed in it or left it.
	HasBeenInRoom bool `json:"has_been_in_room"`
	// True if the user is in room.
	IsInRoom bool `json:"is_in_room"`
}

// QueryMembershipsForRoomRequest is a request to QueryMembershipsForRoom
type QueryMembershipsForRoomRequest struct {
	// If true, only returns the membership events of "join" membership
	JoinedOnly bool `json:"joined_only"`
	// ID of the room to fetch memberships from
	RoomID string `json:"room_id"`
	// ID of the user sending the request
	Sender string `json:"sender"`
}

// QueryMembershipsForRoomResponse is a response to QueryMembershipsForRoom
type QueryMembershipsForRoomResponse struct {
	// The "m.room.member" events (of "join" membership) in the client format
	JoinEvents []gomatrixserverlib.ClientEvent `json:"join_events"`
	// True if the user has been in room before and has either stayed in it or
	// left it.
	HasBeenInRoom bool `json:"has_been_in_room"`
}

// QueryInvitesForUserRequest is a request to QueryInvitesForUser
type QueryInvitesForUserRequest struct {
	// The room ID to look up invites in.
	RoomID string `json:"room_id"`
	// The User ID to look up invites for.
	TargetUserID string `json:"target_user_id"`
}

// QueryInvitesForUserResponse is a response to QueryInvitesForUser
// This is used when accepting an invite or rejecting a invite to tell which
// remote matrix servers to contact.
type QueryInvitesForUserResponse struct {
	// A list of matrix user IDs for each sender of an active invite targeting
	// the requested user ID.
	InviteSenderUserIDs []string `json:"invite_sender_user_ids"`
}

// QueryServerAllowedToSeeEventRequest is a request to QueryServerAllowedToSeeEvent
type QueryServerAllowedToSeeEventRequest struct {
	// The event ID to look up invites in.
	EventID string `json:"event_id"`
	// The server interested in the event
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

// QueryServerAllowedToSeeEventResponse is a response to QueryServerAllowedToSeeEvent
type QueryServerAllowedToSeeEventResponse struct {
	// Wether the server in question is allowed to see the event
	AllowedToSeeEvent bool `json:"can_see_event"`
}

// QueryMissingEventsRequest is a request to QueryMissingEvents
type QueryMissingEventsRequest struct {
	// Events which are known previous to the gap in the timeline.
	EarliestEvents []string `json:"earliest_events"`
	// Latest known events.
	LatestEvents []string `json:"latest_events"`
	// Limit the number of events this query returns.
	Limit int `json:"limit"`
	// The server interested in the event
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

// QueryMissingEventsResponse is a response to QueryMissingEvents
type QueryMissingEventsResponse struct {
	// Missing events, arbritrary order.
	Events []gomatrixserverlib.HeaderedEvent `json:"events"`
}

// QueryStateAndAuthChainRequest is a request to QueryStateAndAuthChain
type QueryStateAndAuthChainRequest struct {
	// The room ID to query the state in.
	RoomID string `json:"room_id"`
	// The list of prev events for the event. Used to calculate the state at
	// the event
	PrevEventIDs []string `json:"prev_event_ids"`
	// The list of auth events for the event. Used to calculate the auth chain
	AuthEventIDs []string `json:"auth_event_ids"`
	// Should state resolution be ran on the result events?
	// TODO: check call sites and remove if we always want to do state res
	ResolveState bool `json:"resolve_state"`
}

// QueryStateAndAuthChainResponse is a response to QueryStateAndAuthChain
type QueryStateAndAuthChainResponse struct {
	// Copy of the request for debugging.
	QueryStateAndAuthChainRequest
	// Does the room exist on this roomserver?
	// If the room doesn't exist this will be false and StateEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// The room version of the room.
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
	// Do all the previous events exist on this roomserver?
	// If some of previous events do not exist this will be false and StateEvents will be empty.
	PrevEventsExist bool `json:"prev_events_exist"`
	// The state and auth chain events that were requested.
	// The lists will be in an arbitrary order.
	StateEvents     []gomatrixserverlib.HeaderedEvent `json:"state_events"`
	AuthChainEvents []gomatrixserverlib.HeaderedEvent `json:"auth_chain_events"`
}

// QueryBackfillRequest is a request to QueryBackfill.
type QueryBackfillRequest struct {
	// The room to backfill
	RoomID string `json:"room_id"`
	// A map of backwards extremity event ID to a list of its prev_event IDs.
	BackwardsExtremities map[string][]string `json:"backwards_extremities"`
	// The maximum number of events to retrieve.
	Limit int `json:"limit"`
	// The server interested in the events.
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

// PrevEventIDs returns the prev_event IDs of all backwards extremities, de-duplicated in a lexicographically sorted order.
func (r *QueryBackfillRequest) PrevEventIDs() []string {
	var prevEventIDs []string
	for _, pes := range r.BackwardsExtremities {
		prevEventIDs = append(prevEventIDs, pes...)
	}
	prevEventIDs = util.UniqueStrings(prevEventIDs)
	return prevEventIDs
}

// QueryBackfillResponse is a response to QueryBackfill.
type QueryBackfillResponse struct {
	// Missing events, arbritrary order.
	Events []gomatrixserverlib.HeaderedEvent `json:"events"`
}

// QueryRoomVersionCapabilitiesRequest asks for the default room version
type QueryRoomVersionCapabilitiesRequest struct{}

// QueryRoomVersionCapabilitiesResponse is a response to QueryRoomVersionCapabilitiesRequest
type QueryRoomVersionCapabilitiesResponse struct {
	DefaultRoomVersion    gomatrixserverlib.RoomVersion            `json:"default"`
	AvailableRoomVersions map[gomatrixserverlib.RoomVersion]string `json:"available"`
}

// QueryRoomVersionForRoomRequest asks for the room version for a given room.
type QueryRoomVersionForRoomRequest struct {
	RoomID string `json:"room_id"`
}

// QueryRoomVersionForRoomResponse is a response to QueryRoomVersionForRoomRequest
type QueryRoomVersionForRoomResponse struct {
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
}

// RoomserverQueryLatestEventsAndStatePath is the HTTP path for the QueryLatestEventsAndState API.
const RoomserverQueryLatestEventsAndStatePath = "/roomserver/queryLatestEventsAndState"

// RoomserverQueryStateAfterEventsPath is the HTTP path for the QueryStateAfterEvents API.
const RoomserverQueryStateAfterEventsPath = "/roomserver/queryStateAfterEvents"

// RoomserverQueryEventsByIDPath is the HTTP path for the QueryEventsByID API.
const RoomserverQueryEventsByIDPath = "/roomserver/queryEventsByID"

// RoomserverQueryMembershipForUserPath is the HTTP path for the QueryMembershipForUser API.
const RoomserverQueryMembershipForUserPath = "/roomserver/queryMembershipForUser"

// RoomserverQueryMembershipsForRoomPath is the HTTP path for the QueryMembershipsForRoom API
const RoomserverQueryMembershipsForRoomPath = "/roomserver/queryMembershipsForRoom"

// RoomserverQueryInvitesForUserPath is the HTTP path for the QueryInvitesForUser API
const RoomserverQueryInvitesForUserPath = "/roomserver/queryInvitesForUser"

// RoomserverQueryServerAllowedToSeeEventPath is the HTTP path for the QueryServerAllowedToSeeEvent API
const RoomserverQueryServerAllowedToSeeEventPath = "/roomserver/queryServerAllowedToSeeEvent"

// RoomserverQueryMissingEventsPath is the HTTP path for the QueryMissingEvents API
const RoomserverQueryMissingEventsPath = "/roomserver/queryMissingEvents"

// RoomserverQueryStateAndAuthChainPath is the HTTP path for the QueryStateAndAuthChain API
const RoomserverQueryStateAndAuthChainPath = "/roomserver/queryStateAndAuthChain"

// RoomserverQueryBackfillPath is the HTTP path for the QueryBackfillPath API
const RoomserverQueryBackfillPath = "/roomserver/queryBackfill"

// RoomserverQueryRoomVersionCapabilitiesPath is the HTTP path for the QueryRoomVersionCapabilities API
const RoomserverQueryRoomVersionCapabilitiesPath = "/roomserver/queryRoomVersionCapabilities"

// RoomserverQueryRoomVersionForRoomPath is the HTTP path for the QueryRoomVersionForRoom API
const RoomserverQueryRoomVersionForRoomPath = "/roomserver/queryRoomVersionForRoom"

// QueryLatestEventsAndState implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryLatestEventsAndState(
	ctx context.Context,
	request *QueryLatestEventsAndStateRequest,
	response *QueryLatestEventsAndStateResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryLatestEventsAndState")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryLatestEventsAndStatePath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAfterEvents implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAfterEvents(
	ctx context.Context,
	request *QueryStateAfterEventsRequest,
	response *QueryStateAfterEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateAfterEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryStateAfterEventsPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryEventsByID implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryEventsByID(
	ctx context.Context,
	request *QueryEventsByIDRequest,
	response *QueryEventsByIDResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryEventsByID")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryEventsByIDPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipForUser implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipForUser(
	ctx context.Context,
	request *QueryMembershipForUserRequest,
	response *QueryMembershipForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMembershipForUser")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMembershipForUserPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMembershipsForRoom implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	request *QueryMembershipsForRoomRequest,
	response *QueryMembershipsForRoomResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMembershipsForRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMembershipsForRoomPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryInvitesForUser implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryInvitesForUser(
	ctx context.Context,
	request *QueryInvitesForUserRequest,
	response *QueryInvitesForUserResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryInvitesForUser")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryInvitesForUserPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryServerAllowedToSeeEvent implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryServerAllowedToSeeEvent(
	ctx context.Context,
	request *QueryServerAllowedToSeeEventRequest,
	response *QueryServerAllowedToSeeEventResponse,
) (err error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryServerAllowedToSeeEvent")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryServerAllowedToSeeEventPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryMissingEvents implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryMissingEvents(
	ctx context.Context,
	request *QueryMissingEventsRequest,
	response *QueryMissingEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryMissingEvents")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryMissingEventsPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryStateAndAuthChain implements RoomserverQueryAPI
func (h *httpRoomserverInternalAPI) QueryStateAndAuthChain(
	ctx context.Context,
	request *QueryStateAndAuthChainRequest,
	response *QueryStateAndAuthChainResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryStateAndAuthChain")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryStateAndAuthChainPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryBackfill implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryBackfill(
	ctx context.Context,
	request *QueryBackfillRequest,
	response *QueryBackfillResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryBackfill")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryBackfillPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryRoomVersionCapabilities implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryRoomVersionCapabilities(
	ctx context.Context,
	request *QueryRoomVersionCapabilitiesRequest,
	response *QueryRoomVersionCapabilitiesResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomVersionCapabilities")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryRoomVersionCapabilitiesPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// QueryRoomVersionForRoom implements RoomServerQueryAPI
func (h *httpRoomserverInternalAPI) QueryRoomVersionForRoom(
	ctx context.Context,
	request *QueryRoomVersionForRoomRequest,
	response *QueryRoomVersionForRoomResponse,
) error {
	if roomVersion, ok := h.immutableCache.GetRoomVersion(request.RoomID); ok {
		response.RoomVersion = roomVersion
		return nil
	}

	span, ctx := opentracing.StartSpanFromContext(ctx, "QueryRoomVersionForRoom")
	defer span.Finish()

	apiURL := h.roomserverURL + RoomserverQueryRoomVersionForRoomPath
	err := internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
	if err == nil {
		h.immutableCache.StoreRoomVersion(request.RoomID, response.RoomVersion)
	}
	return err
}
