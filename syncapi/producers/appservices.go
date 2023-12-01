// Copyright 2023 The Matrix.org Foundation C.I.C.
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
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

// AppserviceEventProducer produces events for the appservice API to consume
type AppserviceEventProducer struct {
	Topic     string
	JetStream nats.JetStreamContext
}

func (a *AppserviceEventProducer) ProduceRoomEvents(
	msg *nats.Msg,
) error {
	if _, err := a.JetStream.PublishMsg(msg); err != nil {
		logrus.WithError(err).Errorf("Failed to produce to topic '%s': %s", a.Topic, err)
		return err
	}
	return nil
}
