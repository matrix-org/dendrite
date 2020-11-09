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

package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
)

// SendTyping sends a typing event to EDU server
func SendTyping(
	ctx context.Context, eduAPI EDUServerInputAPI, userID, roomID string,
	typing bool, timeoutMS int64,
) error {
	requestData := InputTypingEvent{
		UserID:         userID,
		RoomID:         roomID,
		Typing:         typing,
		TimeoutMS:      timeoutMS,
		OriginServerTS: gomatrixserverlib.AsTimestamp(time.Now()),
	}

	var response InputTypingEventResponse
	err := eduAPI.InputTypingEvent(
		ctx, &InputTypingEventRequest{InputTypingEvent: requestData}, &response,
	)

	return err
}

// SendToDevice sends a typing event to EDU server
func SendToDevice(
	ctx context.Context, eduAPI EDUServerInputAPI, sender, userID, deviceID, eventType string,
	message interface{},
) error {
	js, err := json.Marshal(message)
	if err != nil {
		return err
	}
	requestData := InputSendToDeviceEvent{
		UserID:   userID,
		DeviceID: deviceID,
		SendToDeviceEvent: gomatrixserverlib.SendToDeviceEvent{
			Sender:  sender,
			Type:    eventType,
			Content: js,
		},
	}
	request := InputSendToDeviceEventRequest{
		InputSendToDeviceEvent: requestData,
	}
	response := InputSendToDeviceEventResponse{}
	return eduAPI.InputSendToDeviceEvent(ctx, &request, &response)
}

// SendReceipt sends a receipt event to EDU Server
func SendReceipt(
	ctx context.Context,
	eduAPI EDUServerInputAPI, userID, roomID, eventID, receiptType string,
	timestamp gomatrixserverlib.Timestamp,
) error {
	request := InputReceiptEventRequest{
		InputReceiptEvent: InputReceiptEvent{
			UserID:    userID,
			RoomID:    roomID,
			EventID:   eventID,
			Type:      receiptType,
			Timestamp: timestamp,
		},
	}
	response := InputReceiptEventResponse{}
	return eduAPI.InputReceiptEvent(ctx, &request, &response)
}
