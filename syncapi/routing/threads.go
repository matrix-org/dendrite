package routing

import (
	rstypes "github.com/matrix-org/dendrite/roomserver/types"
	"net/http"
	"strconv"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

type ThreadsResponse struct {
	Chunk     []synctypes.ClientEvent `json:"chunk"`
	NextBatch string                  `json:"next_batch,omitempty"`
}

func Threads(
	req *http.Request,
	device *userapi.Device,
	syncDB storage.Database,
	rsAPI api.SyncRoomserverAPI,
	rawRoomID string) util.JSONResponse {
	var err error
	roomID, err := spec.NewRoomID(rawRoomID)
	if err != nil {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: spec.InvalidParam("invalid room ID"),
		}
	}

	limit, err := strconv.ParseUint(req.URL.Query().Get("limit"), 10, 64)
	if err != nil {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	var from types.StreamPosition
	if f := req.URL.Query().Get("from"); f != "" {
		if from, err = types.NewStreamPositionFromString(f); err != nil {
			return util.ErrorResponse(err)
		}
	}

	include := req.URL.Query().Get("include")

	snapshot, err := syncDB.NewDatabaseSnapshot(req.Context())
	if err != nil {
		logrus.WithError(err).Error("Failed to get snapshot for relations")
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(snapshot, &succeeded, &err)

	res := &ThreadsResponse{
		Chunk: []synctypes.ClientEvent{},
	}

	var userID string
	if include == "participated" {
		_, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("device.UserID invalid")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("internal server error"),
			}
		}
		userID = device.UserID
	} else {
		userID = ""
	}
	var headeredEvents []*rstypes.HeaderedEvent
	headeredEvents, _, res.NextBatch, err = snapshot.ThreadsFor(
		req.Context(), roomID.String(), userID, from, limit,
	)
	if err != nil {
		return util.ErrorResponse(err)
	}

	for _, event := range headeredEvents {
		ce, err := synctypes.ToClientEvent(event, synctypes.FormatAll, func(roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
			return rsAPI.QueryUserIDForSender(req.Context(), roomID, senderID)
		})
		if err != nil {
			return util.ErrorResponse(err)
		}
		res.Chunk = append(res.Chunk, *ce)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: res,
	}
}
