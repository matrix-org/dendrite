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
	"fmt"

	"github.com/Shopify/sarama"
	eduapi "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/dendrite/internal"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type CrossSigningKeyUpdateConsumer struct {
	consumer   *internal.ContinualConsumer
	db         storage.Database
	queues     *queue.OutgoingQueues
	serverName gomatrixserverlib.ServerName
	rsAPI      roomserverAPI.RoomserverInternalAPI
}

func NewCrossSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.KeyServer,
	kafkaConsumer sarama.Consumer,
	queues *queue.OutgoingQueues,
	store storage.Database,
	rsAPI roomserverAPI.RoomserverInternalAPI,
) *CrossSigningKeyUpdateConsumer {
	c := &CrossSigningKeyUpdateConsumer{
		consumer: &internal.ContinualConsumer{
			Process:        process,
			ComponentName:  "federationsender/signingkeys",
			Topic:          string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputCrossSigningKeyUpdate)),
			Consumer:       kafkaConsumer,
			PartitionStore: store,
		},
		queues:     queues,
		db:         store,
		serverName: cfg.Matrix.ServerName,
		rsAPI:      rsAPI,
	}
	c.consumer.ProcessMessage = c.onMessage

	return c
}

func (t *CrossSigningKeyUpdateConsumer) Start() error {
	if err := t.consumer.Start(); err != nil {
		return fmt.Errorf("t.consumer.Start: %w", err)
	}
	return nil
}

func (t *CrossSigningKeyUpdateConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	var output eduapi.OutputSigningKeyUpdate
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		logrus.WithError(err).Errorf("eduserver output log: message parse failure")
		return nil
	}
	_, host, err := gomatrixserverlib.SplitID('@', output.UserID)
	if err != nil {
		logrus.WithError(err).Errorf("eduserver output log: user ID parse failure")
		return nil
	}
	if host != gomatrixserverlib.ServerName(t.serverName) {
		// Ignore any messages that didn't originate locally, otherwise we'll
		// end up parroting information we received from other servers.
		return nil
	}
	logger := log.WithField("user_id", output.UserID)

	var queryRes roomserverAPI.QueryRoomsForUserResponse
	err = t.rsAPI.QueryRoomsForUser(context.Background(), &roomserverAPI.QueryRoomsForUserRequest{
		UserID:         output.UserID,
		WantMembership: "join",
	}, &queryRes)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined rooms for user")
		return nil
	}
	// send this key change to all servers who share rooms with this user.
	destinations, err := t.db.GetJoinedHostsForRooms(context.Background(), queryRes.RoomIDs)
	if err != nil {
		logger.WithError(err).Error("failed to calculate joined hosts for rooms user is in")
		return nil
	}

	// Pack the EDU and marshal it
	edu := &gomatrixserverlib.EDU{
		Type:   eduapi.MSigningKeyUpdate,
		Origin: string(t.serverName),
	}
	if edu.Content, err = json.Marshal(output.CrossSigningKeyUpdate); err != nil {
		return err
	}

	logger.Infof("Sending cross-signing update message to %q", destinations)
	return t.queues.SendEDU(edu, t.serverName, destinations)
}
