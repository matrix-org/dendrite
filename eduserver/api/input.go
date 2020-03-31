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
	"net/http"

	commonHTTP "github.com/matrix-org/dendrite/common/http"
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

// InputTypingEventRequest is a request to EDUServerInputAPI
type InputTypingEventRequest struct {
	InputTypingEvent InputTypingEvent `json:"input_typing_event"`
}

// InputTypingEventResponse is a response to InputTypingEvents
type InputTypingEventResponse struct{}

// EDUServerInputAPI is used to write events to the typing server.
type EDUServerInputAPI interface {
	InputTypingEvent(
		ctx context.Context,
		request *InputTypingEventRequest,
		response *InputTypingEventResponse,
	) error
}

// EDUServerInputTypingEventPath is the HTTP path for the InputTypingEvent API.
const EDUServerInputTypingEventPath = "/api/eduserver/input"

// NewEDUServerInputAPIHTTP creates a EDUServerInputAPI implemented by talking to a HTTP POST API.
func NewEDUServerInputAPIHTTP(eduServerURL string, httpClient *http.Client) EDUServerInputAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpEDUServerInputAPI{eduServerURL, httpClient}
}

type httpEDUServerInputAPI struct {
	eduServerURL string
	httpClient   *http.Client
}

// InputRoomEvents implements EDUServerInputAPI
func (h *httpEDUServerInputAPI) InputTypingEvent(
	ctx context.Context,
	request *InputTypingEventRequest,
	response *InputTypingEventResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputTypingEvent")
	defer span.Finish()

	apiURL := h.eduServerURL + EDUServerInputTypingEventPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
