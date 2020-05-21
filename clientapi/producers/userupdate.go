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

	"github.com/Shopify/sarama"
)

// UserUpdateProducer produces events related to user updates.
type UserUpdateProducer struct {
	Topic    string
	Producer sarama.SyncProducer
}

// TODO: Move this struct to `internal` so the components that consume the topic
// can use it when parsing incoming messages
type profileUpdate struct {
	Updated  string `json:"updated"`   // Which attribute is updated (can be either `avatar_url` or `displayname`)
	OldValue string `json:"old_value"` // The attribute's value before the update
	NewValue string `json:"new_value"` // The attribute's value after the update
}

// SendUpdate sends an update using kafka to notify the roomserver of the
// profile update. Returns an error if the update failed to send.
func (p *UserUpdateProducer) SendUpdate(
	userID string, updatedAttribute string, oldValue string, newValue string,
) error {
	var update profileUpdate
	var m sarama.ProducerMessage

	m.Topic = string(p.Topic)
	m.Key = sarama.StringEncoder(userID)

	update = profileUpdate{
		Updated:  updatedAttribute,
		OldValue: oldValue,
		NewValue: newValue,
	}

	value, err := json.Marshal(update)
	if err != nil {
		return err
	}
	m.Value = sarama.ByteEncoder(value)

	_, _, err = p.Producer.SendMessage(&m)
	return err
}
