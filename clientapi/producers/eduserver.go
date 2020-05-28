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
	"encoding/json"
	"time"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// EDUServerProducer produces events for the EDU server to consume
type EDUServerProducer struct {
	InputAPI api.EDUServerInputAPI
}

// NewEDUServerProducer creates a new EDUServerProducer
func NewEDUServerProducer(inputAPI api.EDUServerInputAPI) *EDUServerProducer {
	return &EDUServerProducer{
		InputAPI: inputAPI,
	}
}

// SendTyping sends a typing event to EDU server
func (p *EDUServerProducer) SendTyping(
	ctx context.Context, userID, roomID string,
	typing bool, timeoutMS int64,
) error {
	requestData := api.InputTypingEvent{
		UserID:         userID,
		RoomID:         roomID,
		Typing:         typing,
		TimeoutMS:      timeoutMS,
		OriginServerTS: gomatrixserverlib.AsTimestamp(time.Now()),
	}

	var response api.InputTypingEventResponse
	err := p.InputAPI.InputTypingEvent(
		ctx, &api.InputTypingEventRequest{InputTypingEvent: requestData}, &response,
	)

	return err
}

// SendToDevice sends a typing event to EDU server
func (p *EDUServerProducer) SendToDevice(
	ctx context.Context, userID, deviceID, eventType string,
	message interface{},
) error {
	js, err := json.Marshal(message)
	if err != nil {
		return err
	}
	requestData := api.InputSendToDeviceEvent{
		SendToDeviceEvent: gomatrixserverlib.SendToDeviceEvent{
			UserID:    userID,
			DeviceID:  deviceID,
			EventType: eventType,
			Message:   js,
		},
	}
	request := api.InputSendToDeviceEventRequest{
		InputSendToDeviceEvent: requestData,
	}
	response := api.InputSendToDeviceEventResponse{}
	return p.InputAPI.InputSendToDeviceEvent(ctx, &request, &response)
}
