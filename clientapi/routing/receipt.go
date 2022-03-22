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
	"fmt"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/clientapi/producers"
	"github.com/matrix-org/gomatrixserverlib"

	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

func SetReceipt(req *http.Request, syncProducer *producers.SyncAPIProducer, device *userapi.Device, roomID, receiptType, eventID string) util.JSONResponse {
	timestamp := gomatrixserverlib.AsTimestamp(time.Now())
	logrus.WithFields(logrus.Fields{
		"roomID":      roomID,
		"receiptType": receiptType,
		"eventID":     eventID,
		"userId":      device.UserID,
		"timestamp":   timestamp,
	}).Debug("Setting receipt")

	// currently only m.read is accepted
	if receiptType != "m.read" {
		return util.MessageResponse(400, fmt.Sprintf("receipt type must be m.read not '%s'", receiptType))
	}

	if err := syncProducer.SendReceipt(req.Context(), device.UserID, roomID, eventID, receiptType, timestamp); err != nil {
		return util.ErrorResponse(err)
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
