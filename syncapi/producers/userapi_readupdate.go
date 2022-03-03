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
	"encoding/json"

	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// UserAPIProducer produces events for the user API server to consume
type UserAPIReadProducer struct {
	Topic     string
	JetStream nats.JetStreamContext
}

// SendData sends account data to the user API server
func (p *UserAPIReadProducer) SendReadUpdate(userID, roomID string, readPos, fullyReadPos types.StreamPosition) error {
	m := &nats.Msg{
		Subject: p.Topic,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, userID)
	m.Header.Set(jetstream.RoomID, roomID)

	data := types.ReadUpdate{
		UserID:    userID,
		RoomID:    roomID,
		Read:      readPos,
		FullyRead: fullyReadPos,
	}
	var err error
	m.Data, err = json.Marshal(data)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"user_id":        userID,
		"room_id":        roomID,
		"read_pos":       readPos,
		"fully_read_pos": fullyReadPos,
	}).Tracef("Producing to topic '%s'", p.Topic)

	_, err = p.JetStream.PublishMsg(m)
	return err
}
