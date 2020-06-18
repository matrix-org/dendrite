// Copyright 2017 Vector Creations Ltd
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
	"io/ioutil"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/util"
)

// GetAccountData implements GET /user/{userId}/[rooms/{roomid}/]account_data/{type}
func GetAccountData(
	req *http.Request, userAPI api.UserInternalAPI, device *api.Device,
	userID string, roomID string, dataType string,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	dataReq := api.QueryAccountDataRequest{
		UserID:   userID,
		DataType: dataType,
		RoomID:   roomID,
	}
	dataRes := api.QueryAccountDataResponse{}
	if err := userAPI.QueryAccountData(req.Context(), &dataReq, &dataRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryAccountData failed")
		return util.ErrorResponse(fmt.Errorf("userAPI.QueryAccountData: %w", err))
	}

	var data json.RawMessage
	var ok bool
	if roomID != "" {
		data, ok = dataRes.RoomAccountData[roomID][dataType]
	} else {
		data, ok = dataRes.GlobalAccountData[dataType]
	}
	if ok {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: data,
		}
	}

	return util.JSONResponse{
		Code: http.StatusNotFound,
		JSON: jsonerror.Forbidden("data not found"),
	}
}

// SaveAccountData implements PUT /user/{userId}/[rooms/{roomId}/]account_data/{type}
func SaveAccountData(
	req *http.Request, userAPI api.UserInternalAPI, device *api.Device,
	userID string, roomID string, dataType string, syncProducer *producers.SyncAPIProducer,
) util.JSONResponse {
	if userID != device.UserID {
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden("userID does not match the current user"),
		}
	}

	defer req.Body.Close() // nolint: errcheck

	if req.Body == http.NoBody {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.NotJSON("Content not JSON"),
		}
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("ioutil.ReadAll failed")
		return jsonerror.InternalServerError()
	}

	if !json.Valid(body) {
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON("Bad JSON content"),
		}
	}

	dataReq := api.InputAccountDataRequest{
		UserID:      userID,
		DataType:    dataType,
		RoomID:      roomID,
		AccountData: json.RawMessage(body),
	}
	dataRes := api.InputAccountDataResponse{}
	if err := userAPI.InputAccountData(req.Context(), &dataReq, &dataRes); err != nil {
		util.GetLogger(req.Context()).WithError(err).Error("userAPI.QueryAccountData failed")
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.Forbidden("data not found"),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
