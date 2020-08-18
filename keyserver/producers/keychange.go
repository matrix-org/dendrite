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

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/sirupsen/logrus"
)

// KeyChange produces key change events for the sync API and federation sender to consume
type KeyChange struct {
	Topic    string
	Producer sarama.SyncProducer
	DB       storage.Database
}

// DefaultPartition returns the default partition this process is sending key changes to.
// NB: A keyserver MUST send key changes to only 1 partition or else query operations will
// become inconsistent. Partitions can be sharded (e.g by hash of user ID of key change) but
// then all keyservers must be queried to calculate the entire set of key changes between
// two sync tokens.
func (p *KeyChange) DefaultPartition() int32 {
	return 0
}

// ProduceKeyChanges creates new change events for each key
func (p *KeyChange) ProduceKeyChanges(keys []api.DeviceMessage) error {
	userToDeviceCount := make(map[string]int)
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
		err = p.DB.StoreKeyChange(context.Background(), partition, offset, key.UserID)
		if err != nil {
			return err
		}
		userToDeviceCount[key.UserID]++
	}
	for userID, count := range userToDeviceCount {
		logrus.WithFields(logrus.Fields{
			"user_id":         userID,
			"num_key_changes": count,
		}).Infof("Produced to key change topic '%s'", p.Topic)
	}
	return nil
}
