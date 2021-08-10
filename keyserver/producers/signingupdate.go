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
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/sirupsen/logrus"
)

type SigningKeyUpdate struct {
	Topic    string
	Producer sarama.SyncProducer
}

func (p *SigningKeyUpdate) DefaultPartition() int32 {
	return 0
}

func (p *SigningKeyUpdate) ProduceSigningKeyUpdate(key api.SigningKeyUpdate) error {
	var m sarama.ProducerMessage
	output := &api.OutputSigningKeyUpdate{
		SigningKeyUpdate: key,
	}

	value, err := json.Marshal(output)
	if err != nil {
		return err
	}

	m.Topic = string(p.Topic)
	m.Key = sarama.StringEncoder(key.UserID)
	m.Value = sarama.ByteEncoder(value)

	_, _, err = p.Producer.SendMessage(&m)
	if err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"user_id": key.UserID,
	}).Infof("Produced to cross-signing update topic '%s'", p.Topic)
	return nil
}
