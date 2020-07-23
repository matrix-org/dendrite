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
	"encoding/json"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/sirupsen/logrus"
)

// KeyChange produces key change events for the sync API and federation sender to consume
type KeyChange struct {
	Topic    string
	Producer sarama.SyncProducer
}

// ProduceKeyChanges creates new change events for each key
func (p *KeyChange) ProduceKeyChanges(keys []api.DeviceKeys) error {
	for _, key := range keys {
		var m sarama.ProducerMessage

		value, err := json.Marshal(key)
		if err != nil {
			return err
		}

		m.Topic = string(p.Topic)
		m.Key = sarama.StringEncoder(key.UserID)
		m.Value = sarama.ByteEncoder(value)

		partition, offset, err := p.Producer.SendMessage(&m)
		if err != nil {
			return err
		}
		logrus.WithFields(logrus.Fields{
			"user_id":   key.UserID,
			"device_id": key.DeviceID,
			"partition": partition,
			"offset":    offset,
		}).Infof("Produced to key change topic '%s'", p.Topic)
	}
	return nil
}
