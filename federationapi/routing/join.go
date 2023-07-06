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
		ServerName: request.Destination(),
		RoomID:     roomID.String(),
	}
	res := api.QueryServerJoinedToRoomResponse{}
	if err = rsAPI.QueryServerJoinedToRoom(httpReq.Context(), &req, &res); err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QueryServerJoinedToRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	createJoinTemplate := func(proto *gomatrixserverlib.ProtoEvent) (gomatrixserverlib.PDU, []gomatrixserverlib.PDU, error) {
		identity, signErr := cfg.Matrix.SigningIdentityFor(request.Destination())
		if signErr != nil {
			util.GetLogger(httpReq.Context()).WithError(signErr).Errorf("obtaining signing identity for %s failed", request.Destination())
			return nil, nil, spec.NotFound(fmt.Sprintf("Server name %q does not exist", request.Destination()))
		}

		queryRes := api.QueryLatestEventsAndStateResponse{
			RoomVersion: roomVersion,
		}
		event, signErr := eventutil.QueryAndBuildEvent(httpReq.Context(), proto, identity, time.Now(), rsAPI, &queryRes)
		switch e := signErr.(type) {
		case nil:
		case eventutil.ErrRoomNoExists:
			util.GetLogger(httpReq.Context()).WithError(signErr).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.NotFound("Room does not exist")
		case gomatrixserverlib.BadJSONError:
			util.GetLogger(httpReq.Context()).WithError(signErr).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.BadJSON(e.Error())
		default:
			util.GetLogger(httpReq.Context()).WithError(signErr).Error("eventutil.BuildEvent failed")
			return nil, nil, spec.InternalServerError{}
		}

		stateEvents := make([]gomatrixserverlib.PDU, len(queryRes.StateEvents))
		for i, stateEvent := range queryRes.StateEvents {
			stateEvents[i] = stateEvent.PDU
		}
		return event, stateEvents, nil
	}

	roomQuerier := api.JoinRoomQuerier{
		Roomserver: rsAPI,
	}

	senderID, err := rsAPI.QuerySenderIDForUser(httpReq.Context(), roomID, userID)
	if err != nil {
		util.GetLogger(httpReq.Context()).WithError(err).Error("rsAPI.QuerySenderIDForUser failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if senderID == "" {
		senderID = spec.SenderID(userID.String())
	}

	input := gomatrixserverlib.HandleMakeJoinInput{
		Context:           httpReq.Context(),
		UserID:            userID,
		SenderID:          senderID,
		RoomID:            roomID,
		RoomVersion:       roomVersion,
		RemoteVersions:    remoteVersions,
		RequestOrigin:     request.Origin(),
		LocalServerName:   cfg.Matrix.ServerName,
		LocalServerInRoom: res.RoomExists && res.IsInRoom,
		RoomQuerier:       &roomQuerier,
		UserIDQuerier: func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(httpReq.Context(), roomID, senderID)
		},
		BuildEventTemplate: createJoinTemplate,
	}
	response, internalErr := gomatrixserverlib.HandleMakeJoin(input)
	switch e := internalErr.(type) {
	case nil:
	case spec.InternalServerError:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_join request")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	case spec.MatrixError:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_join request")
		code := http.StatusInternalServerError
		switch e.ErrCode {
		case spec.ErrorForbidden:
			code = http.StatusForbidden
		case spec.ErrorNotFound:
			code = http.StatusNotFound
		case spec.ErrorUnableToAuthoriseJoin:
			fallthrough // http.StatusBadRequest
		case spec.ErrorBadJSON:
			code = http.StatusBadRequest
		}

		return util.JSONResponse{
			Code: code,
			JSON: e,
		}
	case spec.IncompatibleRoomVersionError:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_join request")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: e,
		}
	default:
		util.GetLogger(httpReq.Context()).WithError(internalErr).Error("failed to handle make_join request")
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("unknown error"),
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

	input := gomatrixserverlib.HandleSendJoinInput{
		Context:           httpReq.Context(),
		RoomID:            roomID,
		EventID:           eventID,
		JoinEvent:         request.Content(),
		RoomVersion:       roomVersion,
		RequestOrigin:     request.Origin(),
		LocalServerName:   cfg.Matrix.ServerName,
		KeyID:             cfg.Matrix.KeyID,
		PrivateKey:        cfg.Matrix.PrivateKey,
		Verifier:          keys,
		MembershipQuerier: &api.MembershipQuerier{Roomserver: rsAPI},
		UserIDQuerier: func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(httpReq.Context(), roomID, senderID)
		},
		StoreSenderIDFromPublicID: func(ctx context.Context, senderID spec.SenderID, userIDRaw string, roomID spec.RoomID) error {
			userID, userErr := spec.NewUserID(userIDRaw, true)
			if userErr != nil {
				return userErr
			}
			return rsAPI.StoreUserRoomPublicKey(ctx, senderID, *userID, roomID)
		},
	}
	response, joinErr := gomatrixserverlib.HandleSendJoin(input)
	switch e := joinErr.(type) {
	case nil:
	case spec.InternalServerError:
		util.GetLogger(httpReq.Context()).WithError(joinErr)
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	case spec.MatrixError:
		util.GetLogger(httpReq.Context()).WithError(joinErr)
		code := http.StatusInternalServerError
		switch e.ErrCode {
		case spec.ErrorForbidden:
			code = http.StatusForbidden
		case spec.ErrorNotFound:
			code = http.StatusNotFound
		case spec.ErrorUnsupportedRoomVersion:
			code = http.StatusInternalServerError
		case spec.ErrorBadJSON:
			code = http.StatusBadRequest
		}

		return util.JSONResponse{
			Code: code,
			JSON: e,
		}
	default:
		util.GetLogger(httpReq.Context()).WithError(joinErr)
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.Unknown("unknown error"),
		}
	}

	if response == nil {
		util.GetLogger(httpReq.Context()).Error("gmsl.HandleMakeJoin returned invalid response")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}

	}

	// Fetch the state and auth chain. We do this before we send the events
	// on, in case this fails.
	var stateAndAuthChainResponse api.QueryStateAndAuthChainResponse
	err = rsAPI.QueryStateAndAuthChain(httpReq.Context(), &api.QueryStateAndAuthChainRequest{
		PrevEventIDs: response.JoinEvent.PrevEventIDs(),
		AuthEventIDs: response.JoinEvent.AuthEventIDs(),
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

	// Send the events to the room server.
	// We are responsible for notifying other servers that the user has joined
	// the room, so set SendAsServer to cfg.Matrix.ServerName
	if !response.AlreadyJoined {
		var rsResponse api.InputRoomEventsResponse
		rsAPI.InputRoomEvents(httpReq.Context(), &api.InputRoomEventsRequest{
			InputRoomEvents: []api.InputRoomEvent{
				{
					Kind:          api.KindNew,
					Event:         &types.HeaderedEvent{PDU: response.JoinEvent},
					SendAsServer:  string(cfg.Matrix.ServerName),
					TransactionID: nil,
				},
			},
		}, &rsResponse)
		if rsResponse.ErrMsg != "" {
			util.GetLogger(httpReq.Context()).WithField(logrus.ErrorKey, rsResponse.ErrMsg).Error("SendEvents failed")
			if rsResponse.NotAllowed {
				return util.JSONResponse{
					Code: http.StatusBadRequest,
					JSON: spec.Forbidden(rsResponse.ErrMsg),
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
			Event:       response.JoinEvent.JSON(),
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
