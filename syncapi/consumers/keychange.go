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

package consumers

import (
	"context"
	"encoding/json"

	"github.com/Shopify/sarama"
	currentstateAPI "github.com/matrix-org/dendrite/currentstateserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
)

// OutputKeyChangeEventConsumer consumes events that originated in the key server.
type OutputKeyChangeEventConsumer struct {
	keyChangeConsumer *internal.ContinualConsumer
	db                storage.Database
	serverName        gomatrixserverlib.ServerName // our server name
	currentStateAPI   currentstateAPI.CurrentStateInternalAPI
	// keyAPI            api.KeyInternalAPI
}

// NewOutputKeyChangeEventConsumer creates a new OutputKeyChangeEventConsumer.
// Call Start() to begin consuming from the key server.
func NewOutputKeyChangeEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	currentStateAPI currentstateAPI.CurrentStateInternalAPI,
	store storage.Database,
) *OutputKeyChangeEventConsumer {

	consumer := internal.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputKeyChangeEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputKeyChangeEventConsumer{
		keyChangeConsumer: &consumer,
		db:                store,
		serverName:        cfg.Matrix.ServerName,
		currentStateAPI:   currentStateAPI,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from the key server
func (s *OutputKeyChangeEventConsumer) Start() error {
	return s.keyChangeConsumer.Start()
}

func (s *OutputKeyChangeEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output api.DeviceKeys
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Error("syncapi: failed to unmarshal key change event from key server")
		return err
	}
	// work out who we need to notify about the new key
	var queryRes currentstateAPI.QuerySharedUsersResponse
	err := s.currentStateAPI.QuerySharedUsers(context.Background(), &currentstateAPI.QuerySharedUsersRequest{}, &queryRes)
	if err != nil {
		log.WithError(err).Error("syncapi: failed to QuerySharedUsers for key change event from key server")
		return err
	}
	// TODO: notify users by waking up streams
	return nil
}

// Catchup returns a list of user IDs of users who have changed their device keys between the partition|offset given and now.
// Returns the new offset for this partition.
func (s *OutputKeyChangeEventConsumer) Catchup(parition int32, offset int64) (userIDs []string, newOffset int, err error) {
	//return s.keyAPI.QueryKeyChangeCatchup(ctx, partition, offset)
	return
}
