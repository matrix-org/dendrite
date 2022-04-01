// Copyright 2020 The Matrix.org Foundation C.I.C.
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

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// KeyChange produces key change events for the sync API and federation sender to consume
type KeyChange struct {
	Topic     string
	JetStream nats.JetStreamContext
	DB        storage.Database
}

// ProduceKeyChanges creates new change events for each key
func (p *KeyChange) ProduceKeyChanges(keys []api.DeviceMessage) error {
	userToDeviceCount := make(map[string]int)
	for _, key := range keys {
		id, err := p.DB.StoreKeyChange(context.Background(), key.UserID)
		if err != nil {
			return err
		}
		key.DeviceChangeID = id
		value, err := json.Marshal(key)
		if err != nil {
			return err
		}

		m := &nats.Msg{
			Subject: p.Topic,
			Header:  nats.Header{},
		}
		m.Header.Set(jetstream.UserID, key.UserID)
		m.Data = value

		_, err = p.JetStream.PublishMsg(m)
		if err != nil {
			return err
		}

		userToDeviceCount[key.UserID]++
	}
	for userID, count := range userToDeviceCount {
		logrus.WithFields(logrus.Fields{
			"user_id":         userID,
			"num_key_changes": count,
		}).Tracef("Produced to key change topic '%s'", p.Topic)
	}
	return nil
}

func (p *KeyChange) ProduceSigningKeyUpdate(key api.CrossSigningKeyUpdate) error {
	output := &api.DeviceMessage{
		Type: api.TypeCrossSigningUpdate,
		OutputCrossSigningKeyUpdate: &api.OutputCrossSigningKeyUpdate{
			CrossSigningKeyUpdate: key,
		},
	}

	id, err := p.DB.StoreKeyChange(context.Background(), key.UserID)
	if err != nil {
		return err
	}
	output.DeviceChangeID = id

	value, err := json.Marshal(output)
	if err != nil {
		return err
	}

	m := &nats.Msg{
		Subject: p.Topic,
		Header:  nats.Header{},
	}
	m.Header.Set(jetstream.UserID, key.UserID)
	m.Data = value

	_, err = p.JetStream.PublishMsg(m)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"user_id": key.UserID,
	}).Tracef("Produced to cross-signing update topic '%s'", p.Topic)
	return nil
}
