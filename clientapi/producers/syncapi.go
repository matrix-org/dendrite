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
	json "github.com/json-iterator/go"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/eventutil"
	log "github.com/sirupsen/logrus"
)

// SyncAPIProducer produces events for the sync API server to consume
type SyncAPIProducer struct {
	Topic    string
	Producer sarama.SyncProducer
}

// SendData sends account data to the sync API server
func (p *SyncAPIProducer) SendData(userID string, roomID string, dataType string) error {
	var m sarama.ProducerMessage

	data := eventutil.AccountData{
		RoomID: roomID,
		Type:   dataType,
	}
	value, err := json.ConfigCompatibleWithStandardLibrary.Marshal(data)
	if err != nil {
		return err
	}

	m.Topic = string(p.Topic)
	m.Key = sarama.StringEncoder(userID)
	m.Value = sarama.ByteEncoder(value)
	log.WithFields(log.Fields{
		"user_id":   userID,
		"room_id":   roomID,
		"data_type": dataType,
	}).Infof("Producing to topic '%s'", p.Topic)

	_, _, err = p.Producer.SendMessage(&m)
	return err
}
