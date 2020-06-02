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

package input

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/sirupsen/logrus"
)

// EDUServerInputAPI implements api.EDUServerInputAPI
type EDUServerInputAPI struct {
	// Cache to store the current typing members in each room.
	Cache *cache.EDUCache
	// The kafka topic to output new typing events to.
	OutputTypingEventTopic string
	// The kafka topic to output new send to device events to.
	OutputSendToDeviceEventTopic string
	// kafka producer
	Producer sarama.SyncProducer
	// device database
	DeviceDB devices.Database
	// our server name
	ServerName gomatrixserverlib.ServerName
}

// InputTypingEvent implements api.EDUServerInputAPI
func (t *EDUServerInputAPI) InputTypingEvent(
	ctx context.Context,
	request *api.InputTypingEventRequest,
	response *api.InputTypingEventResponse,
) error {
	ite := &request.InputTypingEvent
	if ite.Typing {
		// user is typing, update our current state of users typing.
		expireTime := ite.OriginServerTS.Time().Add(
			time.Duration(ite.TimeoutMS) * time.Millisecond,
		)
		t.Cache.AddTypingUser(ite.UserID, ite.RoomID, &expireTime)
	} else {
		t.Cache.RemoveUser(ite.UserID, ite.RoomID)
	}

	return t.sendTypingEvent(ite)
}

// InputTypingEvent implements api.EDUServerInputAPI
func (t *EDUServerInputAPI) InputSendToDeviceEvent(
	ctx context.Context,
	request *api.InputSendToDeviceEventRequest,
	response *api.InputSendToDeviceEventResponse,
) error {
	ise := &request.InputSendToDeviceEvent
	return t.sendToDeviceEvent(ise)
}

func (t *EDUServerInputAPI) sendTypingEvent(ite *api.InputTypingEvent) error {
	ev := &api.TypingEvent{
		Type:   gomatrixserverlib.MTyping,
		RoomID: ite.RoomID,
		UserID: ite.UserID,
		Typing: ite.Typing,
	}
	ote := &api.OutputTypingEvent{
		Event: *ev,
	}

	if ev.Typing {
		expireTime := ite.OriginServerTS.Time().Add(
			time.Duration(ite.TimeoutMS) * time.Millisecond,
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

func (t *EDUServerInputAPI) sendToDeviceEvent(ise *api.InputSendToDeviceEvent) error {
	devices := []string{}
	localpart, domain, err := gomatrixserverlib.SplitID('@', ise.UserID)
	if err != nil {
		return err
	}

	// If the event is targeted locally then we want to expand the wildcard
	// out into individual device IDs so that we can send them to each respective
	// device. If the event isn't targeted locally then we can't expand the
	// wildcard as we don't know about the remote devices, so instead we leave it
	// as-is, so that the federation sender can send it on with the wildcard intact.
	if domain == t.ServerName && ise.DeviceID == "*" {
		devs, err := t.DeviceDB.GetDevicesByLocalpart(context.TODO(), localpart)
		if err != nil {
			return err
		}
		for _, dev := range devs {
			devices = append(devices, dev.ID)
		}
	} else {
		devices = append(devices, ise.DeviceID)
	}

	for _, device := range devices {
		ote := &api.OutputSendToDeviceEvent{
			UserID:            ise.UserID,
			DeviceID:          device,
			SendToDeviceEvent: ise.SendToDeviceEvent,
		}

		logrus.WithFields(logrus.Fields{
			"user_id":    ise.UserID,
			"device_id":  ise.DeviceID,
			"event_type": ise.Type,
		}).Info("handling send-to-device message")

		eventJSON, err := json.Marshal(ote)
		if err != nil {
			logrus.WithError(err).Error("sendToDevice failed json.Marshal")
			return err
		}

		m := &sarama.ProducerMessage{
			Topic: string(t.OutputSendToDeviceEventTopic),
			Key:   sarama.StringEncoder(ote.UserID),
			Value: sarama.ByteEncoder(eventJSON),
		}

		_, _, err = t.Producer.SendMessage(m)
		if err != nil {
			logrus.WithError(err).Error("sendToDevice failed t.Producer.SendMessage")
			return err
		}
	}

	return nil
}

// SetupHTTP adds the EDUServerInputAPI handlers to the http.ServeMux.
func (t *EDUServerInputAPI) SetupHTTP(internalAPIMux *mux.Router) {
	internalAPIMux.Handle(api.EDUServerInputTypingEventPath,
		internal.MakeInternalAPI("inputTypingEvents", func(req *http.Request) util.JSONResponse {
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
	internalAPIMux.Handle(api.EDUServerInputSendToDeviceEventPath,
		internal.MakeInternalAPI("inputSendToDeviceEvents", func(req *http.Request) util.JSONResponse {
			var request api.InputSendToDeviceEventRequest
			var response api.InputSendToDeviceEventResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := t.InputSendToDeviceEvent(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}
