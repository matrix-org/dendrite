// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"math"
	"net/http"

	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/syncapi/storage"
	"github.com/element-hq/dendrite/syncapi/synctypes"
	"github.com/element-hq/dendrite/syncapi/types"
	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
)

type getMembershipResponse struct {
	Chunk []synctypes.ClientEvent `json:"chunk"`
}

// GetMemberships implements
//
//	GET /rooms/{roomId}/members
func GetMemberships(
	req *http.Request, device *userapi.Device, roomID string,
	syncDB storage.Database, rsAPI api.SyncRoomserverAPI,
	membership, notMembership *string, at string,
) util.JSONResponse {
	userID, err := spec.NewUserID(device.UserID, true)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("Device UserID is invalid"),
		}
	}
	queryReq := api.QueryMembershipForUserRequest{
		RoomID: roomID,
		UserID: *userID,
	}

	var queryRes api.QueryMembershipForUserResponse
	if queryErr := rsAPI.QueryMembershipForUser(req.Context(), &queryReq, &queryRes); queryErr != nil {
		util.GetLogger(req.Context()).WithError(queryErr).Error("rsAPI.QueryMembershipsForRoom failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	if !queryRes.HasBeenInRoom {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: spec.Forbidden("You aren't a member of the room and weren't previously a member of the room."),
		}
	}

	db, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	defer db.Rollback() // nolint: errcheck

	atToken, err := types.NewTopologyTokenFromString(at)
	if err != nil {
		atToken = types.TopologyToken{Depth: math.MaxInt64, PDUPosition: math.MaxInt64}
		if queryRes.HasBeenInRoom && !queryRes.IsInRoom {
			// If you have left the room then this will be the members of the room when you left.
			atToken, err = db.EventPositionInTopology(req.Context(), queryRes.EventID)
			if err != nil {
				util.GetLogger(req.Context()).WithError(err).Error("unable to get 'atToken'")
				return util.JSONResponse{
					Code: http.StatusInternalServerError,
					JSON: spec.InternalServerError{},
				}
			}
		}
	}

	eventIDs, err := db.SelectMemberships(req.Context(), roomID, atToken, membership, notMembership)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("db.SelectMemberships failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	qryRes := &api.QueryEventsByIDResponse{}
	if err := rsAPI.QueryEventsByID(req.Context(), &api.QueryEventsByIDRequest{EventIDs: eventIDs, RoomID: roomID}, qryRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("rsAPI.QueryEventsByID failed")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}

	result := qryRes.Events

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: getMembershipResponse{synctypes.ToClientEvents(gomatrixserverlib.ToPDUs(result), synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
		})},
	}
}
