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
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
)

type JoinRoomQuerier struct {
	roomserver api.FederationRoomserverAPI
}

func (rq *JoinRoomQuerier) CurrentStateEvent(ctx context.Context, roomID spec.RoomID, eventType string, stateKey string) (gomatrixserverlib.PDU, error) {
	return rq.roomserver.CurrentStateEvent(ctx, roomID, eventType, stateKey)
}

func (rq *JoinRoomQuerier) InvitePending(ctx context.Context, roomID spec.RoomID, userID spec.UserID) (bool, error) {
	return rq.roomserver.InvitePending(ctx, roomID, userID)
}

func (rq *JoinRoomQuerier) RestrictedRoomJoinInfo(ctx context.Context, roomID spec.RoomID, userID spec.UserID, localServerName spec.ServerName) (*gomatrixserverlib.RestrictedRoomJoinInfo, error) {
	roomInfo, err := rq.roomserver.QueryRoomInfo(ctx, roomID)
	if err != nil || roomInfo == nil || roomInfo.IsStub() {
		return nil, err
	}

	req := api.QueryServerJoinedToRoomRequest{
		ServerName: localServerName,
		RoomID:     roomID.String(),
	}
	res := api.QueryServerJoinedToRoomResponse{}
	if err = rq.roomserver.QueryServerJoinedToRoom(ctx, &req, &res); err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return nil, fmt.Errorf("InternalServerError: Failed to query room: %w", err)
	}

	userJoinedToRoom, err := rq.roomserver.UserJoinedToRoom(ctx, types.RoomNID(roomInfo.RoomNID), userID)
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.UserJoinedToRoom failed")
		return nil, fmt.Errorf("InternalServerError: %w", err)
	}

	locallyJoinedUsers, err := rq.roomserver.LocallyJoinedUsers(ctx, roomInfo.RoomVersion, types.RoomNID(roomInfo.RoomNID))
	if err != nil {
		util.GetLogger(ctx).WithError(err).Error("rsAPI.GetLocallyJoinedUsers failed")
		return nil, fmt.Errorf("InternalServerError: %w", err)
	}

	return &gomatrixserverlib.RestrictedRoomJoinInfo{
		LocalServerInRoom: res.RoomExists && res.IsInRoom,
		UserJoinedToRoom:  userJoinedToRoom,
		JoinedUsers:       locallyJoinedUsers,
	}, nil
}

// MakeJoin implements the /make_join API
func MakeJoin(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	roomID spec.RoomID, userID spec.UserID,
	remoteVersions []gomatrixserverlib.RoomVersion,
) util.JSONResponse {
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), roomID.String())
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("failed obtaining room version")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	req := api.QueryServerJoinedToRoomRequest{
		ServerName: cfg.Matrix.ServerName,
		RoomID:     roomID.String(),
	}
	res := api.QueryServerJoinedToRoomResponse{}
	if err := rsAPI.QueryServerJoinedToRoom(httpReq.Context(), &req, &res); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	createJoinTemplate := func(proto *gomatrixserverlib.ProtoEvent) (gomatrixserverlib.PDU, []gomatrixserverlib.PDU, error) {
		identity, err := cfg.Matrix.SigningIdentityFor(request.Destination())
		if err != nil {
			util.GetLogger(httpReq.Context()).WithError(err).Errorf("obtaining signing identity for %s failed", request.Destination())
			return nil, nil, spec.NotFound(fmt.Sprintf("Server name %q does not exist", request.Destination()))
		}

		queryRes := api.QueryLatestEventsAndStateResponse{
			RoomVersion: roomVersion,
		}
		event, err := eventutil.QueryAndBuildEvent(httpReq.Context(), proto, cfg.Matrix, identity, time.Now(), rsAPI, &queryRes)
		switch e := err.(type) {
		case nil:
		case eventutil.ErrRoomNoExists:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.NotFound("Room does not exist")
		case gomatrixserverlib.BadJSONError:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.BadJSON(e.Error())
		default:
			util.GetLogger(httpReq.Context()).WithError(err).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.InternalServerError{}
		}

		stateEvents := make([]gomatrixserverlib.PDU, len(queryRes.StateEvents))
		for i, stateEvent := range queryRes.StateEvents {
			stateEvents[i] = stateEvent.PDU
		}
		return event, stateEvents, nil
	}

	roomQuerier := JoinRoomQuerier{
		roomserver: rsAPI,
	}

	input := gomatrixserverlib.HandleMakeJoinInput{
		Context:            httpReq.Context(),
		UserID:             userID,
		RoomID:             roomID,
		RoomVersion:        roomVersion,
		RemoteVersions:     remoteVersions,
		RequestOrigin:      request.Origin(),
		LocalServerName:    cfg.Matrix.ServerName,
		LocalServerInRoom:  res.RoomExists && res.IsInRoom,
		RoomQuerier:        &roomQuerier,
		BuildEventTemplate: createJoinTemplate,
	}
	response, internalErr := gomatrixserverlib.HandleMakeJoin(input)
	if internalErr != nil {
		switch e := internalErr.(type) {
		case nil:
		case spec.InternalServerError:
			util.GetLogger(httpReq.Context()).WithError(internalErr)
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		case spec.MatrixError:
			util.GetLogger(httpReq.Context()).WithError(internalErr)
			code := http.StatusInternalServerError
			switch e.ErrCode {
			case spec.ErrorForbidden:
				code = http.StatusForbidden
			case spec.ErrorNotFound:
				code = http.StatusNotFound
			case spec.ErrorUnableToAuthoriseJoin:
				code = http.StatusBadRequest
			case spec.ErrorBadJSON:
				code = http.StatusBadRequest
			}

			return util.JSONResponse{
				Code: code,
				JSON: e,
			}
		case spec.IncompatibleRoomVersionError:
			util.GetLogger(httpReq.Context()).WithError(internalErr)
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: e,
			}
		default:
			util.GetLogger(httpReq.Context()).WithError(internalErr)
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown("unknown error"),
			}
		}
	}

	if response == nil {
		util.GetLogger(httpReq.Context()).Error("gmsl.HandleMakeJoin returned invalid response")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"event":        response.JoinTemplateEvent,
			"room_version": response.RoomVersion,
		},
	}
}

// SendJoin implements the /send_join API
// The make-join send-join dance makes much more sense as a single
// flow so the cyclomatic complexity is high:
// nolint:gocyclo
func SendJoin(
	httpReq *http.Request,
	request *fclient.FederationRequest,
	cfg *config.FederationAPI,
	rsAPI api.FederationRoomserverAPI,
	keys gomatrixserverlib.JSONVerifier,
	roomID spec.RoomID,
	eventID string,
) util.JSONResponse {
	roomVersion, err := rsAPI.QueryRoomVersionForRoom(httpReq.Context(), roomID.String())
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryRoomVersionForRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	verImpl, err := gomatrixserverlib.GetRoomVersion(roomVersion)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.UnsupportedRoomVersion(
				fmt.Sprintf("QueryRoomVersionForRoom returned unknown room version: %s", roomVersion),
			),
		}
	}

	event, err := verImpl.NewEventFromUntrustedJSON(request.Content())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The request body could not be decoded into valid JSON: " + err.Error()),
		}
	}

	// Check that a state key is provided.
	if event.StateKey() == nil || event.StateKeyEquals("") {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("No state key was provided in the join event."),
		}
	}
	if !event.StateKeyEquals(event.Sender()) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("Event state key must match the event sender."),
		}
	}

	// Check that the sender belongs to the server that is sending us
	// the request. By this point we've already asserted that the sender
	// and the state key are equal so we don't need to check both.
	sender, err := spec.NewUserID(event.Sender(), true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("The sender of the join is invalid"),
		}
	} else if sender.Domain() != request.Origin() {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("The sender does not match the server that originated the request"),
		}
	}

	// Check that the room ID is correct.
	if event.RoomID() != roomID.String() {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(
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
			JSON: spec.BadJSON(
				fmt.Sprintf(
					"The event ID in the request path (%q) must match the event ID in the join event JSON (%q)",
					eventID, event.EventID(),
				),
			),
		}
	}

	// Check that this is in fact a join event
	membership, err := event.Membership()
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("missing content.membership key"),
		}
	}
	if membership != spec.Join {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("membership must be 'join'"),
		}
	}

	// Check that the event is signed by the server sending the request.
	redacted, err := verImpl.RedactEventJSON(event.JSON())
	if err != nil {
		logrus.WithError(err).Errorf("XXX: join.go")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON("The event JSON could not be redacted"),
		}
	}
	verifyRequests := []gomatrixserverlib.VerifyJSONRequest{{
		ServerName:             sender.Domain(),
		Message:                redacted,
		AtTS:                   event.OriginServerTS(),
		StrictValidityChecking: true,
	}}
	verifyResults, err := keys.VerifyJSONs(httpReq.Context(), verifyRequests)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("keys.VerifyJSONs failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if verifyResults[0].Error != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("Signature check failed: " + verifyResults[0].Error.Error()),
		}
	}

	// Fetch the state and auth chain. We do this before we send the events
	// on, in case this fails.
	var stateAndAuthChainResponse api.QueryStateAndAuthChainResponse
	err = rsAPI.QueryStateAndAuthChain(httpReq.Context(), &api.QueryStateAndAuthChainRequest{
		PrevEventIDs: event.PrevEventIDs(),
		AuthEventIDs: event.AuthEventIDs(),
		RoomID:       roomID.String(),
		ResolveState: true,
	}, &stateAndAuthChainResponse)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryStateAndAuthChain failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !stateAndAuthChainResponse.RoomExists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Room does not exist"),
		}
	}
	if !stateAndAuthChainResponse.StateKnown {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("State not known"),
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
			alreadyJoined = (membership == spec.Join)
			isBanned = (membership == spec.Ban)
			break
		}
	}

	if isBanned {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("user is banned"),
		}
	}

	// If the membership content contains a user ID for a server that is not
	// ours then we should kick it back.
	var memberContent gomatrixserverlib.MemberContent
	if err := json.Unmarshal(event.Content(), &memberContent); err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.BadJSON(err.Error()),
		}
	}
	if memberContent.AuthorisedVia != "" {
		_, domain, err := gomatrixserverlib.SplitID('@', memberContent.AuthorisedVia)
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON(fmt.Sprintf("The authorising username %q is invalid.", memberContent.AuthorisedVia)),
			}
		}
		if domain != cfg.Matrix.ServerName {
			return util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.BadJSON(fmt.Sprintf("The authorising username %q does not belong to this server.", memberContent.AuthorisedVia)),
			}
		}
	}

	// Sign the membership event. This is required for restricted joins to work
	// in the case that the authorised via user is one of our own users. It also
	// doesn't hurt to do it even if it isn't a restricted join.
	signed := event.Sign(
		string(cfg.Matrix.ServerName),
		cfg.Matrix.KeyID,
		cfg.Matrix.PrivateKey,
	)

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has joined
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	if !alreadyJoined {
		var response api.InputRoomEventsResponse
		rsAPI.InputRoomEvents(httpReq.Context(), &api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{
				{
					Kind:          api.KindNew,
					Event:         &types.HeaderedEvent{PDU: signed},
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
					JSON: spec.Forbidden(response.ErrMsg),
				}
			}
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
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
		JSON: fclient.RespSendJoin{
			StateEvents: types.NewEventJSONsFromHeaderedEvents(stateAndAuthChainResponse.StateEvents),
			AuthEvents:  types.NewEventJSONsFromHeaderedEvents(stateAndAuthChainResponse.AuthChainEvents),
			Origin:      cfg.Matrix.ServerName,
			Event:       signed.JSON(),
		},
	}
}

type eventsByDepth []*types.HeaderedEvent

func (e eventsByDepth) Len() int {
	return len(e)
}
func (e eventsByDepth) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}
func (e eventsByDepth) Less(i, j int) bool {
	return e[i].Depth() < e[j].Depth()
}
