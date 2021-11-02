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
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// MakeJoin implements the /make_join API
func MakeJoin(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.RoomserverInternalAPI,
	roomID, userID string,
	remoteVersions []gomatrixserverlib.RoomVersion,
) util.JSONResponse {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), &verReq, &verRes); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.InternalServerError(),
		}
	}

	// Check that the room that the remote side is trying to join is actually
	// one of the room versions that they listed in their supported ?ver= in
	// the make_join URL.
	// https://matrix.org/docs/spec/server_server/r0.1.3#get-matrix-federation-v1-make-join-roomid-userid
	remoteSupportsVersion := false
	for _, v := range remoteVersions {
		if v == verRes.RoomVersion {
			remoteSupportsVersion = true
			break
		}
	}
	// If it isn't, stop trying to join the room.
	if !remoteSupportsVersion {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.IncompatibleRoomVersion(verRes.RoomVersion),
		}
	}

	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Invalid UserID"),
		}
	}
	if domain != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The join must be sent by the server of the user"),
		}
	}

	// Check if we think we are still joined to the room
	inRoomReq := &api.QueryServerJoinedToRoomRequest{
		ServerName: cfg.Matrix.ServerName,
		RoomID:     roomID,
	}
	inRoomRes := &api.QueryServerJoinedToRoomResponse{}
	if err = rsAPI.QueryServerJoinedToRoom(httpReq.Context(), inRoomReq, inRoomRes); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return jsonerror.InternalServerError()
	}
	if !inRoomRes.RoomExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(fmt.Sprintf("Room ID %q was not found on this server", roomID)),
		}
	}
	if !inRoomRes.IsInRoom {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(fmt.Sprintf("Room ID %q has no remaining users on this server", roomID)),
		}
	}

	// Try building an event for the server
	builder := gomatrixserverlib.EventBuilder{
		Sender:   userID,
		RoomID:   roomID,
		Type:     "m.room.member",
		StateKey: &userID,
	}
	err = builder.SetContent(map[string]interface{}{"membership": gomatrixserverlib.Join})
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("builder.SetContent failed")
		return jsonerror.InternalServerError()
	}

	queryRes := api.QueryLatestEventsAndStateResponse{
		RoomVersion: verRes.RoomVersion,
	}
	event, err := eventutil.QueryAndBuildEvent(httpReq.Context(), &builder, cfg.Matrix, time.Now(), rsAPI, &queryRes)
	if err == eventutil.ErrRoomNoExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	} else if e, ok := err.(gomatrixserverlib.BadJSONError); ok {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(e.Error()),
		}
	} else if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
		return jsonerror.InternalServerError()
	}

	// Check that the join is allowed or not
	stateEvents := make([]*gomatrixserverlib.Event, len(queryRes.StateEvents))
	for i := range queryRes.StateEvents {
		stateEvents[i] = queryRes.StateEvents[i].Event
	}
	provider := gomatrixserverlib.NewAuthEvents(stateEvents)

	// Check the join rules. If it's a restricted join then there are special rules.
	var joinRuleEvent *gomatrixserverlib.Event
	var joinRules gomatrixserverlib.JoinRuleContent
	if joinRuleEvent, err = provider.JoinRules(); err != nil {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room join rules do not exist"),
		}
	} else if err = json.Unmarshal(joinRuleEvent.Content(), &joinRules); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown("Failed to unmarshal room join rules"),
		}
	}

	if err = gomatrixserverlib.Allowed(event.Event, &provider); err != nil {
		if joinRules.JoinRule == gomatrixserverlib.Restricted {
			return attemptMakeJoinForRestrictedMembership(
				httpReq, cfg, rsAPI, &verRes,
				provider, &builder, joinRules, userID,
			)
		}
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(err.Error()),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"event":        builder,
			"room_version": verRes.RoomVersion,
		},
	}
}

func attemptMakeJoinForRestrictedMembership(
	httpReq *http.Request,
	cfg *config.FederationAPI,
	rsAPI api.RoomserverInternalAPI,
	verRes *api.QueryRoomVersionForRoomResponse,
	provider gomatrixserverlib.AuthEvents,
	builder *gomatrixserverlib.EventBuilder,
	joinRules gomatrixserverlib.JoinRuleContent,
	userID string,
) util.JSONResponse {
	// As a last effort, see if any of the restricted join rules match.
	// If so, we might be able to modify and sign the event so that it
	// does pass auth.
	var powerLevels gomatrixserverlib.PowerLevelContent
	if powerLevelsEvent, err := provider.PowerLevels(); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnableToAuthoriseJoin("Room power levels do not exist"),
		}
	} else if err := json.Unmarshal(powerLevelsEvent.Content(), &powerLevels); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.UnableToAuthoriseJoin("Failed to unmarshal room power levels"),
		}
	}

	// Let's see if we can validate the user being in
	// any of the allowed rooms.
	for _, allowed := range joinRules.Allow {
		// Skip types that we don't know about.
		if allowed.Type != gomatrixserverlib.MRoomMembership {
			continue
		}

		// Ask the room server if we know about the specified room ID.
		queryReq := &api.QueryMembershipsForRoomRequest{
			RoomID:     allowed.RoomID,
			JoinedOnly: true,
		}
		queryRes := &api.QueryMembershipsForRoomResponse{}
		if err := rsAPI.QueryMembershipsForRoom(httpReq.Context(), queryReq, queryRes); err != nil {
			continue
		}

		// Now have a look and see if any of the joined users match the
		// user who has initiated this join.
		found := false
		for _, member := range queryRes.JoinEvents {
			if member.StateKey == nil {
				continue // shouldn't ever happen
			}
			if *member.StateKey == userID {
				found = true
				break
			}
		}

		// The user doesn't seem to exist in this room, try the next one.
		if !found {
			continue
		}

		// Now look through all of the join events of the other members. Our goal
		// is to try and find a user from our own server that has a suitable power
		// level to popuate into the `join_authorised_via_users_server` field.
		for _, member := range queryRes.JoinEvents {
			if member.StateKey == nil {
				continue // shouldn't ever happen
			}

			// If the user doesn't come from our own server then it's no good, try
			// the next one instead.
			_, domain, err := gomatrixserverlib.SplitID('@', *member.StateKey)
			if err != nil {
				continue
			}
			if domain != cfg.Matrix.ServerName {
				continue
			}

			// If the user has the ability to invite to the room then they are a
			// suitable candidate for the `join_authorised_via_users_server`.
			if powerLevels.UserLevel(*member.StateKey) >= powerLevels.Invite {
				// We'll set the event content again, this time including the
				// `join_authorised_via_users_server` field for the chosen user.
				err := builder.SetContent(map[string]interface{}{
					"membership":                       gomatrixserverlib.Join,
					"join_authorised_via_users_server": *member.StateKey,
				})
				if err != nil {
					util.GetLogger(httpReq.Context()).WithError(err).Error("builder.SetContent failed")
					return jsonerror.InternalServerError()
				}

				// Then we'll build the event again. This is a second hit on the
				// roomserver sadly, but it's a necessary evil.
				queryRes := api.QueryLatestEventsAndStateResponse{
					RoomVersion: verRes.RoomVersion,
				}
				event, err := eventutil.QueryAndBuildEvent(httpReq.Context(), builder, cfg.Matrix, time.Now(), rsAPI, &queryRes)
				if err != nil {
					util.GetLogger(httpReq.Context()).WithError(err).Error("builder.SetContent failed")
					return jsonerror.InternalServerError()
				}

				// Sign the event. This is basically our seal of approval that
				// other servers can use to verify that the user we put into the
				// `join_authorised_via_users_server` field was actually checked
				// and found by us.
				signed := event.Sign(string(cfg.Matrix.ServerName), cfg.Matrix.KeyID, cfg.Matrix.PrivateKey)

				// Now, see if the join is valid with the new changes. If it isn't
				// then something else is forbidding the join.
				if err = gomatrixserverlib.Allowed(&signed, &provider); err != nil {
					return util.JSONResponse{
						Code: http.StatusForbidden,
						JSON: jsonerror.Forbidden(err.Error()),
					}
				}

				// Otherwise, the new join event looks good, so return it.
				return util.JSONResponse{
					Code: http.StatusOK,
					JSON: map[string]interface{}{
						"event":        signed,
						"room_version": verRes.RoomVersion,
					},
				}
			}
		}
	}

	return util.JSONResponse{
		Code: http.StatusBadRequest,
		JSON: jsonerror.UnableToAuthoriseJoin("You are not joined to any allowed rooms"),
	}
}

// SendJoin implements the /send_join API
// The make-join send-join dance makes much more sense as a single
// flow so the cyclomatic complexity is high:
func SendJoin(
	httpReq *http.Request,
	request *gomatrixserverlib.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.RoomserverInternalAPI,
	keys gomatrixserverlib.JSONVerifier,
	roomID, eventID string,
) util.JSONResponse {
	verReq := api.QueryRoomVersionForRoomRequest{RoomID: roomID}
	verRes := api.QueryRoomVersionForRoomResponse{}
	if err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), &verReq, &verRes); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryRoomVersionForRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: jsonerror.InternalServerError(),
		}
	}

	event, err := gomatrixserverlib.NewEventFromUntrustedJSON(request.Content(), verRes.RoomVersion)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("The request body could not be decoded into valid JSON: " + err.Error()),
		}
	}

	// Check that a state key is provided.
	if event.StateKey() == nil || event.StateKeyEquals("") {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("No state key was provided in the join event."),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(
				fmt.Sprintf(
					"The room ID in the request path (%q) must match the room ID in the join event JSON (%q)",
					roomID, event.RoomID(),
				),
			),
		}
	}

	// Check that the event ID is correct.
	if event.EventID() != eventID {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(
				fmt.Sprintf(
					"The event ID in the request path (%q) must match the event ID in the join event JSON (%q)",
					eventID, event.EventID(),
				),
			),
		}
	}

	// Check that the event is from the server sending the request.
	if event.Origin() != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("The join must be sent by the server it originated on"),
		}
	}

	// Check that this is in fact a join event
	membership, err := event.Membership()
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("missing content.membership key"),
		}
	}
	if membership != gomatrixserverlib.Join {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("membership must be 'join'"),
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted := event.Redact()
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName:             event.Origin(),
		Message:                redacted.JSON(),
		AtTS:                   event.OriginServerTS(),
		StrictValidityChecking: true,
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("keys.VerifyJSONs failed")
		return jsonerror.InternalServerError()
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("Signature check failed: " + verifyResults[0].Error.Error()),
		}
	}

	// Fetch the state and auth chain. We do this before we send the events
	// on, in case this fails.
	var stateAndAuthChainResponse api.QueryStateAndAuthChainResponse
	err = rsAPI.QueryStateAndAuthChain(httpReq.Context(), &api.QueryStateAndAuthChainRequest{
		PrevEventIDs: event.PrevEventIDs(),
		AuthEventIDs: event.AuthEventIDs(),
		RoomID:       roomID,
		ResolveState: true,
	}, &stateAndAuthChainResponse)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryStateAndAuthChain failed")
		return jsonerror.InternalServerError()
	}

	if !stateAndAuthChainResponse.RoomExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("Room does not exist"),
		}
	}

	// Check if the user is already in the room. If they're already in then
	// there isn't much point in sending another join event into the room.
	// Also check to see if they are banned: if they are then we reject them.
	alreadyJoined := false
	isBanned := false
	for _, se := range stateAndAuthChainResponse.StateEvents {
		if !se.StateKeyEquals(*event.StateKey()) {
			continue
		}
		if membership, merr := se.Membership(); merr == nil {
			alreadyJoined = (membership == gomatrixserverlib.Join)
			isBanned = (membership == gomatrixserverlib.Ban)
			break
		}
	}

	if isBanned {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("user is banned"),
		}
	}

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has joined
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	if !alreadyJoined {
		var response api.InputRoomEventsResponse
		rsAPI.InputRoomEvents(httpReq.Context(), &api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{
				{
					Kind:          api.KindNew,
					Event:         event.Headered(stateAndAuthChainResponse.RoomVersion),
					AuthEventIDs:  event.AuthEventIDs(),
					SendAsServer:  string(cfg.Matrix.ServerName),
					TransactionID: nil,
				},
			},
		}, &response)
		if response.ErrMsg != "" {
			util.GetLogger(httpReq.Context()).WithField(logrus.ErrorKey, response.ErrMsg).Error("SendEvents failed")
			if response.NotAllowed {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: jsonerror.Forbidden(response.ErrMsg),
				}
			}
			return jsonerror.InternalServerError()
		}
	}

	// sort events deterministically by depth (lower is earlier)
	// We also do this because sytest's basic federation server isn't good at using the correct
	// state if these lists are randomised, resulting in flakey tests. :(
	sort.Sort(eventsByDepth(stateAndAuthChainResponse.StateEvents))
	sort.Sort(eventsByDepth(stateAndAuthChainResponse.AuthChainEvents))

	// https://matrix.org/docs/spec/server_server/latest#put-matrix-federation-v1-send-join-roomid-eventid
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: gomatrixserverlib.RespSendJoin{
			StateEvents: gomatrixserverlib.UnwrapEventHeaders(stateAndAuthChainResponse.StateEvents),
			AuthEvents:  gomatrixserverlib.UnwrapEventHeaders(stateAndAuthChainResponse.AuthChainEvents),
			Origin:      cfg.Matrix.ServerName,
		},
	}
}

type eventsByDepth []*gomatrixserverlib.HeaderedEvent

func (e eventsByDepth) Len() int {
	return len(e)
}
func (e eventsByDepth) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}
func (e eventsByDepth) Less(i, j int) bool {
	return e[i].Depth() < e[j].Depth()
}
