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

package producers

import (
	"context"
	"time"

	"github.com/matrix-org/dendrite/typingserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// TypingServerProducer produces events for the typing server to consume
type TypingServerProducer struct {
	InputAPI api.TypingServerInputAPI
}

// NewTypingServerProducer creates a new TypingServerProducer
func NewTypingServerProducer(inputAPI api.TypingServerInputAPI) *TypingServerProducer {
	return &TypingServerProducer{
		InputAPI: inputAPI,
	}
}

// Send typing event to typing server
func (p *TypingServerProducer) Send(
	ctx context.Context, userID, roomID string,
	typing bool, timeout int64,
) error {
	data := api.InputTypingEvent{
		UserID:         userID,
		RoomID:         roomID,
		Typing:         typing,
		Timeout:        timeout,
		OriginServerTS: gomatrixserverlib.AsTimestamp(time.Now()),
	}
	request := []api.InputTypingEvent{data}

	var response api.InputTypingEventsResponse
	err := p.InputAPI.InputTypingEvents(
		ctx, &api.InputTypingEventsRequest{InputTypingEvents: request}, &response,
	)

	return err
}
