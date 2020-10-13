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
	"net/http"

	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

func SetReceipt(req *http.Request, rsAPI roomserverAPI.RoomserverInternalAPI, device *userapi.Device, roomId, receiptType, eventId string) util.JSONResponse {
	logrus.WithFields(logrus.Fields{
		"roomId":      roomId,
		"receiptType": receiptType,
		"eventId":     eventId,
		"userId":      device.UserID,
	}).Debug("Setting receipt")
	userReq := &roomserverAPI.PerformUserReceiptUpdateRequest{
		RoomID:      roomId,
		ReceiptType: receiptType,
		EventID:     eventId,
		UserID:      device.UserID,
	}
	userResp := &roomserverAPI.PerformUserReceiptUpdateResponse{}

	if err := rsAPI.PerformUserReceiptUpdate(req.Context(), userReq, userResp); err != nil {
		return util.ErrorResponse(err)
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
