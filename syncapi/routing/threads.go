package routing

import (
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/synctypes"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
)

type ThreadsResponse struct {
	Chunk     []synctypes.ClientEvent `json:"chunk"`
	NextBatch string                  `json:"next_batch,omitempty"`
}

func Threads(
	req *http.Request,
	device userapi.Device,
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

	from := req.URL.Query().Get("from")
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

	if include == "participated" {
		userID, err := spec.NewUserID(device.UserID, true)
		if err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("device.UserID invalid")
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.Unknown("internal server error"),
			}
		}
		var events []types.StreamEvent
		events, res.PrevBatch, res.NextBatch, err = snapshot.RelationsFor(
			req.Context(), roomID.String(), "", relType, eventType, from, to, dir == "b", limit,
		)
	}
}
