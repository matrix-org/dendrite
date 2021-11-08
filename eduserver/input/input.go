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
	"time"

	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
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
	// The kafka topic to output new receipt events to
	OutputReceiptEventTopic string
	// The kafka topic to output new key change events to
	OutputKeyChangeEventTopic string
	// kafka producer
	JetStream nats.JetStreamContext
	// Internal user query API
	UserAPI userapi.UserInternalAPI
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

// InputCrossSigningKeyUpdate implements api.EDUServerInputAPI
func (t *EDUServerInputAPI) InputCrossSigningKeyUpdate(
	ctx context.Context,
	request *api.InputCrossSigningKeyUpdateRequest,
	response *api.InputCrossSigningKeyUpdateResponse,
) error {
	eventJSON, err := json.Marshal(&keyapi.DeviceMessage{
		Type: keyapi.TypeCrossSigningUpdate,
		OutputCrossSigningKeyUpdate: &api.OutputCrossSigningKeyUpdate{
			CrossSigningKeyUpdate: request.CrossSigningKeyUpdate,
		},
	})
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"user_id": request.UserID,
	}).Infof("Producing to topic '%s'", t.OutputKeyChangeEventTopic)

	_, err = t.JetStream.PublishMsg(&nats.Msg{
		Subject: t.OutputKeyChangeEventTopic,
		Header:  nats.Header{},
		Data:    eventJSON,
	})
	return err
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
	logrus.WithFields(logrus.Fields{
		"room_id": ite.RoomID,
		"user_id": ite.UserID,
		"typing":  ite.Typing,
	}).Infof("Producing to topic '%s'", t.OutputTypingEventTopic)

	_, err = t.JetStream.PublishMsg(&nats.Msg{
		Subject: t.OutputTypingEventTopic,
		Header:  nats.Header{},
		Data:    eventJSON,
	})
	return err
}

func (t *EDUServerInputAPI) sendToDeviceEvent(ise *api.InputSendToDeviceEvent) error {
	devices := []string{}
	_, domain, err := gomatrixserverlib.SplitID('@', ise.UserID)
	if err != nil {
		return err
	}

	// If the event is targeted locally then we want to expand the wildcard
	// out into individual device IDs so that we can send them to each respective
	// device. If the event isn't targeted locally then we can't expand the
	// wildcard as we don't know about the remote devices, so instead we leave it
	// as-is, so that the federation sender can send it on with the wildcard intact.
	if domain == t.ServerName && ise.DeviceID == "*" {
		var res userapi.QueryDevicesResponse
		err = t.UserAPI.QueryDevices(context.TODO(), &userapi.QueryDevicesRequest{
			UserID: ise.UserID,
		}, &res)
		if err != nil {
			return err
		}
		for _, dev := range res.Devices {
			devices = append(devices, dev.ID)
		}
	} else {
		devices = append(devices, ise.DeviceID)
	}

	logrus.WithFields(logrus.Fields{
		"user_id":     ise.UserID,
		"num_devices": len(devices),
		"type":        ise.Type,
	}).Infof("Producing to topic '%s'", t.OutputSendToDeviceEventTopic)
	for _, device := range devices {
		ote := &api.OutputSendToDeviceEvent{
			UserID:            ise.UserID,
			DeviceID:          device,
			SendToDeviceEvent: ise.SendToDeviceEvent,
		}

		eventJSON, err := json.Marshal(ote)
		if err != nil {
			logrus.WithError(err).Error("sendToDevice failed json.Marshal")
			return err
		}

		if _, err = t.JetStream.PublishMsg(&nats.Msg{
			Subject: t.OutputSendToDeviceEventTopic,
			Data:    eventJSON,
		}); err != nil {
			logrus.WithError(err).Error("sendToDevice failed t.Producer.SendMessage")
			return err
		}
	}

	return nil
}

// InputReceiptEvent implements api.EDUServerInputAPI
// TODO: Intelligently batch requests sent by the same user (e.g wait a few milliseconds before emitting output events)
func (t *EDUServerInputAPI) InputReceiptEvent(
	ctx context.Context,
	request *api.InputReceiptEventRequest,
	response *api.InputReceiptEventResponse,
) error {
	logrus.WithFields(logrus.Fields{}).Infof("Producing to topic '%s'", t.OutputReceiptEventTopic)
	output := &api.OutputReceiptEvent{
		UserID:    request.InputReceiptEvent.UserID,
		RoomID:    request.InputReceiptEvent.RoomID,
		EventID:   request.InputReceiptEvent.EventID,
		Type:      request.InputReceiptEvent.Type,
		Timestamp: request.InputReceiptEvent.Timestamp,
	}
	js, err := json.Marshal(output)
	if err != nil {
		return err
	}

	_, err = t.JetStream.PublishMsg(&nats.Msg{
		Subject: t.OutputReceiptEventTopic,
		Data:    js,
	})
	return err
}
