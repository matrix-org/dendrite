package writers

import (
	"encoding/json"
	"net/http"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/common"
	"github.com/matrix-org/util"
)

// http://matrix.org/docs/spec/client_server/r0.2.0.html#post-matrix-client-r0-createroom
type createRoomRequest struct {
	Invite          []string               `json:"invite"`
	Name            string                 `json:"name"`
	Visibility      string                 `json:"visibility"`
	Topic           string                 `json:"topic"`
	Preset          string                 `json:"preset"`
	CreationContent map[string]interface{} `json:"creation_content"`
	InitialState    json.RawMessage        `json:"initial_state"` // TODO
	RoomAliasName   string                 `json:"room_alias_name"`
}

// CreateRoom implements /createRoom
func CreateRoom(req *http.Request) util.JSONResponse {
	logger := util.GetLogger(req.Context())
	userID, resErr := auth.VerifyAccessToken(req)
	if resErr != nil {
		return *resErr
	}
	var r createRoomRequest
	resErr = common.UnmarshalJSONRequest(req, &r)
	if resErr != nil {
		return *resErr
	}

	logger.WithFields(log.Fields{
		"userID": userID,
	}).Info("Creating room")
	return util.MessageResponse(404, "Not implemented yet")
}
