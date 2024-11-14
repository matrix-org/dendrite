// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"net/http"
	"time"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/clientapi/httputil"
	"github.com/element-hq/dendrite/internal/eventutil"
	roomserverAPI "github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

func JoinRoomByIDOrAlias(
	req *http.Request,
	device *api.Device,
	rsAPI roomserverAPI.ClientRoomserverAPI,
	profileAPI api.ClientUserAPI,
	roomIDOrAlias string,
) util.JSONResponse {
	// Prepare to ask the roomserver to perform the room join.
	joinReq := roomserverAPI.PerformJoinRequest{
		RoomIDOrAlias: roomIDOrAlias,
		UserID:        device.UserID,
		IsGuest:       device.AccountType == api.AccountTypeGuest,
		Content:       map[string]interface{}{},
	}

	// Check to see if any ?via= or ?server_name= query parameters
	// were given in the request.
	if serverNames, ok := req.URL.Query()["via"]; ok {
		for _, serverName := range serverNames {
			joinReq.ServerNames = append(
				joinReq.ServerNames,
				spec.ServerName(serverName),
			)
		}
	} else if serverNames, ok := req.URL.Query()["server_name"]; ok {
		for _, serverName := range serverNames {
			joinReq.ServerNames = append(
				joinReq.ServerNames,
				spec.ServerName(serverName),
			)
		}
	}

	// If content was provided in the request then include that
	// in the request. It'll get used as a part of the membership
	// event content.
	_ = httputil.UnmarshalJSONRequest(req, &joinReq.Content)

	// Work out our localpart for the client profile request.

	// Request our profile content to populate the request content with.
	profile, err := profileAPI.QueryProfile(req.Context(), device.UserID)

	switch err {
	case nil:
		joinReq.Content["displayname"] = profile.DisplayName
		joinReq.Content["avatar_url"] = profile.AvatarURL
	case appserviceAPI.ErrProfileNotExists:
		util.GetLogger(req.Context()).Error("Unable to query user profile, no profile found.")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.Unknown("Unable to query user profile, no profile found."),
		}
	default:
	}

	// Ask the roomserver to perform the join.
	done := make(chan util.JSONResponse, 1)
	go func() {
		defer close(done)
		roomID, _, err := rsAPI.PerformJoin(req.Context(), &joinReq)
		var response util.JSONResponse

		switch e := err.(type) {
		case nil: // success case
			response = util.JSONResponse{
				Code: http.StatusOK,
				// TODO: Put the response struct somewhere internal.
				JSON: struct {
					RoomID string `json:"room_id"`
				}{roomID},
			}
		case roomserverAPI.ErrInvalidID:
			response = util.JSONResponse{
				Code: http.StatusBadRequest,
				JSON: spec.Unknown(e.Error()),
			}
		case roomserverAPI.ErrNotAllowed:
			jsonErr := spec.Forbidden(e.Error())
			if device.AccountType == api.AccountTypeGuest {
				jsonErr = spec.GuestAccessForbidden(e.Error())
			}
			response = util.JSONResponse{
				Code: http.StatusForbidden,
				JSON: jsonErr,
			}
		case *gomatrix.HTTPError: // this ensures we proxy responses over federation to the client
			response = util.JSONResponse{
				Code: e.Code,
				JSON: json.RawMessage(e.Message),
			}
		case eventutil.ErrRoomNoExists:
			response = util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: spec.NotFound(e.Error()),
			}
		default:
			response = util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}
		done <- response
	}()

	// Wait either for the join to finish, or for us to hit a reasonable
	// timeout, at which point we'll just return a 200 to placate clients.
	timer := time.NewTimer(time.Second * 20)
	select {
	case <-timer.C:
		return util.JSONResponse{
			Code: http.StatusAccepted,
			JSON: spec.Unknown("The room join will continue in the background."),
		}
	case result := <-done:
		// Stop and drain the timer
		if !timer.Stop() {
			<-timer.C
		}
		return result
	}
}
