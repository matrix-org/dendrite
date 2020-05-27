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
	"encoding/json"
	"errors"
	"net/http"

	internalHTTP "github.com/matrix-org/dendrite/internal/http"
	"github.com/matrix-org/gomatrixserverlib"
	opentracing "github.com/opentracing/opentracing-go"
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
	// The user ID to send the update to.
	UserID string `json:"user_id"`
	// The device ID to send the update to.
	DeviceID string `json:"device_id"`
	// The type of the event.
	EventType string `json:"event_type"`
	// The contents of the message.
	Message json.RawMessage `json:"message"`
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
}

// EDUServerInputTypingEventPath is the HTTP path for the InputTypingEvent API.
const EDUServerInputTypingEventPath = "/eduserver/input"

// EDUServerInputSendToDeviceEventPath is the HTTP path for the InputSendToDeviceEvent API.
const EDUServerInputSendToDeviceEventPath = "/eduserver/sendToDevice"

// NewEDUServerInputAPIHTTP creates a EDUServerInputAPI implemented by talking to a HTTP POST API.
func NewEDUServerInputAPIHTTP(eduServerURL string, httpClient *http.Client) (EDUServerInputAPI, error) {
	if httpClient == nil {
		return nil, errors.New("NewTypingServerInputAPIHTTP: httpClient is <nil>")
	}
	return &httpEDUServerInputAPI{eduServerURL, httpClient}, nil
}

type httpEDUServerInputAPI struct {
	eduServerURL string
	httpClient   *http.Client
}

// InputTypingEvent implements EDUServerInputAPI
func (h *httpEDUServerInputAPI) InputTypingEvent(
	ctx context.Context,
	request *InputTypingEventRequest,
	response *InputTypingEventResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputTypingEvent")
	defer span.Finish()

	apiURL := h.eduServerURL + EDUServerInputTypingEventPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}

// InputSendToDeviceEvent implements EDUServerInputAPI
func (h *httpEDUServerInputAPI) InputSendToDeviceEvent(
	ctx context.Context,
	request *InputSendToDeviceEventRequest,
	response *InputSendToDeviceEventResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputSendToDeviceEvent")
	defer span.Finish()

	apiURL := h.eduServerURL + EDUServerInputSendToDeviceEventPath
	return internalHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
