// Copyright 2017 Vector Creations Ltd
// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

// Package api provides the types that are used to communicate with the typing server.
package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

// InputTypingEvent is an event for notifying the typing server about typing updates.
type InputTypingEvent struct {
	// UserID of the user to update typing status.
	UserID string `json:"user_id"`
	// RoomID of the room the user is typing (or has stopped).
	RoomID string `json:"room_id"`
	// Typing is true if the user is typing, false if they have stopped.
	Typing bool `json:"typing"`
	// Timeout is the interval in milliseconds for which the user should be marked as typing.
	TimeoutMS int64 `json:"timeout"`
	// OriginServerTS when the server received the update.
	OriginServerTS gomatrixserverlib.Timestamp `json:"origin_server_ts"`
}

type InputSendToDeviceEvent struct {
	UserID   string `json:"user_id"`
	DeviceID string `json:"device_id"`
	gomatrixserverlib.SendToDeviceEvent
}

// InputTypingEventRequest is a request to EDUServerInputAPI
type InputTypingEventRequest struct {
	InputTypingEvent InputTypingEvent `json:"input_typing_event"`
}

// InputTypingEventResponse is a response to InputTypingEvents
type InputTypingEventResponse struct{}

// InputSendToDeviceEventRequest is a request to EDUServerInputAPI
type InputSendToDeviceEventRequest struct {
	InputSendToDeviceEvent InputSendToDeviceEvent `json:"input_send_to_device_event"`
}

// InputSendToDeviceEventResponse is a response to InputSendToDeviceEventRequest
type InputSendToDeviceEventResponse struct{}

type InputReceiptEvent struct {
	UserID    string                      `json:"user_id"`
	RoomID    string                      `json:"room_id"`
	EventID   string                      `json:"event_id"`
	Type      string                      `json:"type"`
	Timestamp gomatrixserverlib.Timestamp `json:"timestamp"`
}

// InputReceiptEventRequest is a request to EDUServerInputAPI
type InputReceiptEventRequest struct {
	InputReceiptEvent InputReceiptEvent `json:"input_receipt_event"`
}

// InputReceiptEventResponse is a response to InputReceiptEventRequest
type InputReceiptEventResponse struct{}

// EDUServerInputAPI is used to write events to the typing server.
type EDUServerInputAPI interface {
	InputTypingEvent(
		ctx context.Context,
		request *InputTypingEventRequest,
		response *InputTypingEventResponse,
	) error

	InputSendToDeviceEvent(
		ctx context.Context,
		request *InputSendToDeviceEventRequest,
		response *InputSendToDeviceEventResponse,
	) error

	InputReceiptEvent(
		ctx context.Context,
		request *InputReceiptEventRequest,
		response *InputReceiptEventResponse,
	) error
}
