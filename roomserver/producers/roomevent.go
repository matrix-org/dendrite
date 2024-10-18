// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package producers

import (
	"encoding/json"

	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"

	"github.com/element-hq/dendrite/roomserver/acls"
	"github.com/element-hq/dendrite/roomserver/api"
	"github.com/element-hq/dendrite/setup/jetstream"
)

var keyContentFields = map[string]string{
	"m.room.join_rules":         "join_rule",
	"m.room.history_visibility": "history_visibility",
	"m.room.member":             "membership",
}

type RoomEventProducer struct {
	Topic     string
	ACLs      *acls.ServerACLs
	JetStream nats.JetStreamContext
}

func (r *RoomEventProducer) ProduceRoomEvents(roomID string, updates []api.OutputEvent) error {
	var err error
	for _, update := range updates {
		msg := nats.NewMsg(r.Topic)
		msg.Header.Set(jetstream.RoomEventType, string(update.Type))
		msg.Header.Set(jetstream.RoomID, roomID)
		msg.Data, err = json.Marshal(update)
		if err != nil {
			return err
		}
		logger := log.WithFields(log.Fields{
			"room_id": roomID,
			"type":    update.Type,
		})
		if update.NewRoomEvent != nil {
			eventType := update.NewRoomEvent.Event.Type()
			logger = logger.WithFields(log.Fields{
				"event_type":     eventType,
				"event_id":       update.NewRoomEvent.Event.EventID(),
				"adds_state":     len(update.NewRoomEvent.AddsStateEventIDs),
				"removes_state":  len(update.NewRoomEvent.RemovesStateEventIDs),
				"send_as_server": update.NewRoomEvent.SendAsServer,
				"sender":         update.NewRoomEvent.Event.SenderID(),
			})
			if update.NewRoomEvent.Event.StateKey() != nil {
				logger = logger.WithField("state_key", *update.NewRoomEvent.Event.StateKey())
			}
			contentKey := keyContentFields[eventType]
			if contentKey != "" {
				value := gjson.GetBytes(update.NewRoomEvent.Event.Content(), contentKey)
				if value.Exists() {
					logger = logger.WithField("content_value", value.String())
				}
			}

			if eventType == acls.MRoomServerACL && update.NewRoomEvent.Event.StateKeyEquals("") {
				ev := update.NewRoomEvent.Event.PDU
				strippedEvent := tables.StrippedEvent{
					RoomID:       ev.RoomID().String(),
					EventType:    ev.Type(),
					StateKey:     *ev.StateKey(),
					ContentValue: string(ev.Content()),
				}
				defer r.ACLs.OnServerACLUpdate(strippedEvent)
			}
		}
		logger.Tracef("Producing to topic '%s'", r.Topic)
		if _, err := r.JetStream.PublishMsg(msg); err != nil {
			logger.WithError(err).Errorf("Failed to produce to topic '%s': %s", r.Topic, err)
			return err
		}
	}
	return nil
}
