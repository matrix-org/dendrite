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
	// Timeout is the interval for which the user should be marked as typing.
	Timeout int64 `json:"timeout"`
	// OriginServerTS when the server received the update.
	OriginServerTS gomatrixserverlib.Timestamp `json:"origin_server_ts"`
}

// InputTypingEventsRequest is a request to TypingServerInputAPI
type InputTypingEventsRequest struct {
	InputTypingEvents []InputTypingEvent `json:"input_typing_events"`
}

// InputTypingEventsResponse is a response to InputTypingEvents
type InputTypingEventsResponse struct{}

// TypingServerInputAPI is used to write events to the typing server.
type TypingServerInputAPI interface {
	InputTypingEvents(
		ctx context.Context,
		request *InputTypingEventsRequest,
		response *InputTypingEventsResponse,
	) error
}

// TypingServerInputTypingEventsPath is the HTTP path for the InputTypingEvents API.
const TypingServerInputTypingEventsPath = "/api/typingserver/input"

// NewTypingServerInputAPIHTTP creates a TypingServerInputAPI implemented by talking to a HTTP POST API.
func NewTypingServerInputAPIHTTP(typingServerURL string, httpClient *http.Client) TypingServerInputAPI {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &httpTypingServerInputAPI{typingServerURL, httpClient}
}

type httpTypingServerInputAPI struct {
	typingServerURL string
	httpClient      *http.Client
}

// InputRoomEvents implements TypingServerInputAPI
func (h *httpTypingServerInputAPI) InputTypingEvents(
	ctx context.Context,
	request *InputTypingEventsRequest,
	response *InputTypingEventsResponse,
) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "InputTypingEvents")
	defer span.Finish()

	apiURL := h.typingServerURL + TypingServerInputTypingEventsPath
	return commonHTTP.PostJSON(ctx, span, h.httpClient, apiURL, request, response)
}
