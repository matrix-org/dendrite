// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/element-hq/dendrite/clientapi/producers"
	"github.com/matrix-org/gomatrixserverlib/spec"

	userapi "github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

func SetReceipt(req *http.Request, userAPI userapi.ClientUserAPI, syncProducer *producers.SyncAPIProducer, device *userapi.Device, roomID, receiptType, eventID string) util.JSONResponse {
	timestamp := spec.AsTimestamp(time.Now())
	logrus.WithFields(logrus.Fields{
		"roomID":      roomID,
		"receiptType": receiptType,
		"eventID":     eventID,
		"userId":      device.UserID,
		"timestamp":   timestamp,
	}).Debug("Setting receipt")

	switch receiptType {
	case "m.read", "m.read.private":
		if err := syncProducer.SendReceipt(req.Context(), device.UserID, roomID, eventID, receiptType, timestamp); err != nil {
			return util.ErrorResponse(err)
		}

	case "m.fully_read":
		data, err := json.Marshal(fullyReadEvent{EventID: eventID})
		if err != nil {
			return util.JSONResponse{
				Code: http.StatusInternalServerError,
				JSON: spec.InternalServerError{},
			}
		}

		dataReq := userapi.InputAccountDataRequest{
			UserID:      device.UserID,
			DataType:    "m.fully_read",
			RoomID:      roomID,
			AccountData: data,
		}
		dataRes := userapi.InputAccountDataResponse{}
		if err := userAPI.InputAccountData(req.Context(), &dataReq, &dataRes); err != nil {
			util.GetLogger(req.Context()).WithError(err).Error("userAPI.InputAccountData failed")
			return util.ErrorResponse(err)
		}

	default:
		return util.MessageResponse(400, fmt.Sprintf("Receipt type '%s' not known", receiptType))
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: struct{}{},
	}
}
