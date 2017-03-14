package writers

import (
	"net/http"

	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/util"
)

// http://matrix.org/docs/spec/client_server/r0.2.0.html#put-matrix-client-r0-rooms-roomid-send-eventtype-txnid
type sendMessageResponse struct {
	EventID string `json:"event_id"`
}

// SendMessage implements /rooms/{roomID}/send/{eventType}/{txnID}
func SendMessage(req *http.Request, roomID, eventType, txnID string, cfg config.ClientAPI, queryAPI api.RoomserverQueryAPI) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	var r map[string]interface{} // must be a JSON object
	resErr = httputil.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}
	eventID := fmt.Sprintf("$%s:%s", util.RandomString(16), cfg.ServerName)

	queryReq := api.QueryLatestEventsAndStateRequest{
		RoomID: roomID,
		StateToFetch: []common.StateKeyTuple{
			{"m.room.member", userID},
		},
	}
	var queryRes api.QueryLatestEventsAndStateResponse
	if err := queryAPI.QueryLatestEventsAndState(&queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	logger.WithFields(log.Fields{
		"roomID":    roomID,
		"eventType": eventType,
		"userID":    userID,
		"res":       queryRes,
	}).Info("Doing stuff...")
	return util.JSONResponse{
		Code: 200,
		JSON: sendMessageResponse{eventID},
	}
}
