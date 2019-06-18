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

package input

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/typingserver/api"
	"github.com/matrix-org/dendrite/typingserver/cache"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"gopkg.in/Shopify/sarama.v1"
)

// TypingServerInputAPI implements api.TypingServerInputAPI
type TypingServerInputAPI struct {
	// Cache to store the current typing members in each room.
	Cache *cache.TypingCache
	// The kafka topic to output new typing events to.
	OutputTypingEventTopic string
	// kafka producer
	Producer sarama.SyncProducer
}

// InputTypingEvent implements api.TypingServerInputAPI
func (t *TypingServerInputAPI) InputTypingEvent(
	ctx context.Context,
	request *api.InputTypingEventRequest,
	response *api.InputTypingEventResponse,
) error {
	ite := &request.InputTypingEvent
	if ite.Typing {
		// user is typing, update our current state of users typing.
		expireTime := ite.OriginServerTS.Time().Add(
			time.Duration(ite.Timeout) * time.Millisecond,
		)
		t.Cache.AddTypingUser(ite.UserID, ite.RoomID, &expireTime)
	} else {
		t.Cache.RemoveUser(ite.UserID, ite.RoomID)
	}

	return t.sendEvent(ite)
}

func (t *TypingServerInputAPI) sendEvent(ite *api.InputTypingEvent) error {
	userIDs := t.Cache.GetTypingUsers(ite.RoomID)
	ev := &api.TypingEvent{
		Type:   gomatrixserverlib.MTyping,
		RoomID: ite.RoomID,
		UserID: ite.UserID,
	}
	ote := &api.OutputTypingEvent{
		Event:       *ev,
		TypingUsers: userIDs,
	}

	if ev.Typing {
		expireTime := ite.OriginServerTS.Time().Add(
			time.Duration(ite.Timeout) * time.Millisecond,
		)
		ote.ExpireTime = &expireTime
	}

	eventJSON, err := json.Marshal(ote)
	if err != nil {
		return err
	}

	m := &sarama.ProducerMessage{
		Topic: string(t.OutputTypingEventTopic),
		Key:   sarama.StringEncoder(ite.RoomID),
		Value: sarama.ByteEncoder(eventJSON),
	}

	_, _, err = t.Producer.SendMessage(m)
	return err
}

// SetupHTTP adds the TypingServerInputAPI handlers to the http.ServeMux.
func (t *TypingServerInputAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.TypingServerInputTypingEventPath,
		common.MakeInternalAPI("inputTypingEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputTypingEventRequest
			var response api.InputTypingEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := t.InputTypingEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
