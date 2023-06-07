// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"errors"
	"net/http"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/transactions"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type redactionContent struct {
	Reason string `json:"reason"`
}

type redactionResponse struct {
	EventID string `json:"event_id"`
}

func SendRedaction(
	req *http.Request, device *userapi.Device, roomID, eventID string, cfg *config.ClientAPI,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	txnID *string,
	txnCache *transactions.Cache,
) util.JSONResponse {
	resErr := checkMemberInRoom(req.Context(), rsAPI, device.UserID, roomID)
	if resErr != nil {
		return *resErr
	}

	if txnID != nil {
		// Try to fetch response from transactionsCache
		if res, ok := txnCache.FetchTransaction(device.AccessToken, *txnID, req.URL); ok {
			return *res
		}
	}

	ev := roomserverAPI.GetEvent(req.Context(), rsAPI, roomID, eventID)
	if ev == nil {
		return util.JSONResponse{
			Code: 400,
			JSON: spec.NotFound("unknown event ID"), // TODO: is it ok to leak existence?
		}
	}
	if ev.RoomID() != roomID {
		return util.JSONResponse{
			Code: 400,
			JSON: spec.NotFound("cannot redact event in another room"),
		}
	}

	fullUserID, userIDErr := spec.NewUserID(device.UserID, true)
	if userIDErr != nil || fullUserID == nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID doesn't have power level to redact"),
		}
	}
	senderID, queryErr := rsAPI.QuerySenderIDForUser(req.Context(), roomID, *fullUserID)
	if queryErr != nil {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("userID doesn't have power level to redact"),
		}
	}

	// "Users may redact their own events, and any user with a power level greater than or equal
	// to the redact power level of the room may redact events there"
	// https://matrix.org/docs/spec/client_server/r0.6.1#put-matrix-client-r0-rooms-roomid-redact-eventid-txnid
	allowedToRedact := ev.SenderID() == senderID // TODO: Should replace device.UserID with device...PerRoomKey
	if !allowedToRedact {
		plEvent := roomserverAPI.GetStateEvent(req.Context(), rsAPI, roomID, gomatrixserverlib.StateKeyTuple{
			EventType: spec.MRoomPowerLevels,
			StateKey:  "",
		})
		if plEvent == nil {
			return util.JSONResponse{
				Code: 403,
				JSON: spec.Forbidden("You don't have permission to redact this event, no power_levels event in this room."),
			}
		}
		pl, err := plEvent.PowerLevels()
		if err != nil {
			return util.JSONResponse{
				Code: 403,
				JSON: spec.Forbidden(
					"You don't have permission to redact this event, the power_levels event for this room is malformed so auth checks cannot be performed.",
				),
			}
		}
		allowedToRedact = pl.UserLevel(senderID) >= pl.Redact
	}
	if !allowedToRedact {
		return util.JSONResponse{
			Code: 403,
			JSON: spec.Forbidden("You don't have permission to redact this event, power level too low."),
		}
	}

	var r redactionContent
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	// create the new event and set all the fields we can
	proto := gomatrixserverlib.ProtoEvent{
		SenderID: string(senderID),
		RoomID:   roomID,
		Type:     spec.MRoomRedaction,
		Redacts:  eventID,
	}
	err := proto.SetContent(r)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("proto.SetContent failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	identity, err := cfg.Matrix.SigningIdentityFor(device.UserDomain())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	var queryRes roomserverAPI.QueryLatestEventsAndStateResponse
	e, err := eventutil.QueryAndBuildEvent(req.Context(), &proto, identity, time.Now(), rsAPI, &queryRes)
	if errors.Is(err, eventutil.ErrRoomNoExists{}) {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("Room does not exist"),
		}
	}
	domain := device.UserDomain()
	if err = roomserverAPI.SendEvents(context.Background(), rsAPI, roomserverAPI.KindNew, []*types.HeaderedEvent{e}, device.UserDomain(), domain, domain, nil, false); err != nil {
		util.GetLogger(req.Context()).WithError(err).Errorf("failed to SendEvents")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	res := util.JSONResponse{
		Code: 200,
		JSON: redactionResponse{
			EventID: e.EventID(),
		},
	}

	// Add response to transactionsCache
	if txnID != nil {
		txnCache.AddTransaction(device.AccessToken, *txnID, req.URL, &res)
	}

	return res
}
