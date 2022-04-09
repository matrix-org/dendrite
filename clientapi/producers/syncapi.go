// Copyright 2017 Vector Creations Ltd
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

package producers

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// SyncAPIProducer produces events for the sync API server to consume
type SyncAPIProducer struct {
	TopicClientData        string
	TopicReceiptEvent      string
	TopicSendToDeviceEvent string
	TopicTypingEvent       string
	TopicPresenceEvent     string
	JetStream              nats.JetStreamContext
	ServerName             gomatrixserverlib.ServerName
	UserAPI                userapi.UserInternalAPI
}

// SendData sends account data to the sync API server
func (p *SyncAPIProducer) SendData(userID string, roomID string, dataType string, readMarker *eventutil.ReadMarkerJSON, ignoredUsers *types.IgnoredUsers) error {
	m := &nats.Msg{
		Subject: p.TopicClientData,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)

	data := eventutil.AccountData{
		RoomID:       roomID,
		Type:         dataType,
		ReadMarker:   readMarker,
		IgnoredUsers: ignoredUsers,
	}
	var err error
	m.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   roomID,
		"data_type": dataType,
	}).Tracef("Producing to topic '%s'", p.TopicClientData)

	_, err = p.JetStream.PublishMsg(m)
	return err
}

func (p *SyncAPIProducer) SendReceipt(
	ctx context.Context,
	userID, roomID, eventID, receiptType string, timestamp gomatrixserverlib.Timestamp,
) error {
	m := &nats.Msg{
		Subject: p.TopicReceiptEvent,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)
	m.Header.Set(jetstream.RoomID, roomID)
	m.Header.Set(jetstream.EventID, eventID)
	m.Header.Set("type", receiptType)
	m.Header.Set("timestamp", strconv.Itoa(int(timestamp)))

	log.WithFields(log.Fields{}).Tracef("Producing to topic '%s'", p.TopicReceiptEvent)
	_, err := p.JetStream.PublishMsg(m, nats.Context(ctx))
	return err
}

func (p *SyncAPIProducer) SendToDevice(
	ctx context.Context, sender, userID, deviceID, eventType string,
	message interface{},
) error {
	devices := []string{}
	_, domain, err := gomatrixserverlib.SplitID('@', userID)
	if err != nil {
		return err
	}

	// If the event is targeted locally then we want to expand the wildcard
	// out into individual device IDs so that we can send them to each respective
	// device. If the event isn't targeted locally then we can't expand the
	// wildcard as we don't know about the remote devices, so instead we leave it
	// as-is, so that the federation sender can send it on with the wildcard intact.
	if domain == p.ServerName && deviceID == "*" {
		var res userapi.QueryDevicesResponse
		err = p.UserAPI.QueryDevices(context.TODO(), &userapi.QueryDevicesRequest{
			UserID: userID,
		}, &res)
		if err != nil {
			return err
		}
		for _, dev := range res.Devices {
			devices = append(devices, dev.ID)
		}
	} else {
		devices = append(devices, deviceID)
	}

	js, err := json.Marshal(message)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id":     userID,
		"num_devices": len(devices),
		"type":        eventType,
	}).Tracef("Producing to topic '%s'", p.TopicSendToDeviceEvent)
	for _, device := range devices {
		ote := &types.OutputSendToDeviceEvent{
			UserID:   userID,
			DeviceID: device,
			SendToDeviceEvent: gomatrixserverlib.SendToDeviceEvent{
				Sender:  sender,
				Type:    eventType,
				Content: js,
			},
		}

		eventJSON, err := json.Marshal(ote)
		if err != nil {
			log.WithError(err).Error("sendToDevice failed json.Marshal")
			return err
		}
		m := &nats.Msg{
			Subject: p.TopicSendToDeviceEvent,
			Data:    eventJSON,
			Header:  nats.Header{},
		}
		m.Header.Set("sender", sender)
		m.Header.Set(jetstream.UserID, userID)
		if _, err = p.JetStream.PublishMsg(m, nats.Context(ctx)); err != nil {
			log.WithError(err).Error("sendToDevice failed t.Producer.SendMessage")
			return err
		}
	}
	return nil
}

func (p *SyncAPIProducer) SendTyping(
	ctx context.Context, userID, roomID string, typing bool, timeoutMS int64,
) error {
	m := &nats.Msg{
		Subject: p.TopicTypingEvent,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)
	m.Header.Set(jetstream.RoomID, roomID)
	m.Header.Set("typing", strconv.FormatBool(typing))
	m.Header.Set("timeout_ms", strconv.Itoa(int(timeoutMS)))

	_, err := p.JetStream.PublishMsg(m, nats.Context(ctx))
	return err
}

func (p *SyncAPIProducer) SendPresence(
	ctx context.Context, userID string, presence types.Presence, statusMsg *string,
) error {
	m := nats.NewMsg(p.TopicPresenceEvent)
	m.Header.Set(jetstream.UserID, userID)
	m.Header.Set("presence", presence.String())
	if statusMsg != nil {
		m.Header.Set("status_msg", *statusMsg)
	}

	m.Header.Set("last_active_ts", strconv.Itoa(int(gomatrixserverlib.AsTimestamp(time.Now()))))

	_, err := p.JetStream.PublishMsg(m, nats.Context(ctx))
	return err
}
