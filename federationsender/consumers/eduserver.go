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

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/federationsender/queue"
	"github.com/matrix-org/dendrite/federationsender/storage"
	"github.com/matrix-org/gomatrixserverlib"
	log "github.com/sirupsen/logrus"
	"gopkg.in/Shopify/sarama.v1"
)

// OutputTypingEventConsumer consumes events that originate in EDU server.
type OutputTypingEventConsumer struct {
	consumer   *common.ContinualConsumer
	db         storage.Database
	queues     *queue.OutgoingQueues
	ServerName gomatrixserverlib.ServerName
}

// NewOutputTypingEventConsumer creates a new OutputTypingEventConsumer. Call Start() to begin consuming from EDU servers.
func NewOutputTypingEventConsumer(
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	queues *queue.OutgoingQueues,
	store storage.Database,
) *OutputTypingEventConsumer {
	consumer := common.ContinualConsumer{
		Topic:          string(cfg.Kafka.Topics.OutputTypingEvent),
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	c := &OutputTypingEventConsumer{
		consumer:   &consumer,
		queues:     queues,
		db:         store,
		ServerName: cfg.Matrix.ServerName,
	}
	consumer.ProcessMessage = c.onMessage

	return c
}

// Start consuming from EDU servers
func (t *OutputTypingEventConsumer) Start() error {
	return t.consumer.Start()
}

// onMessage is called for OutputTypingEvent received from the EDU servers.
// Parses the msg, creates a matrix federation EDU and sends it to joined hosts.
func (t *OutputTypingEventConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	// Extract the typing event from msg.
	var ote api.OutputTypingEvent
	if err := json.Unmarshal(msg.Value, &ote); err != nil {
		// Skip this msg but continue processing messages.
		log.WithError(err).Errorf("eduserver output log: message parse failed")
		return nil
	}

	// only send typing events which originated from us
	_, typingServerName, err := gomatrixserverlib.SplitID('@', ote.Event.UserID)
	if err != nil {
		log.WithError(err).WithField("user_id", ote.Event.UserID).Error("Failed to extract domain from typing sender")
		return nil
	}
	if typingServerName != t.ServerName {
		log.WithField("other_server", typingServerName).Info("Suppressing typing notif: originated elsewhere")
		return nil
	}

	joined, err := t.db.GetJoinedHosts(context.TODO(), ote.Event.RoomID)
	if err != nil {
		return err
	}

	names := make([]gomatrixserverlib.ServerName, len(joined))
	for i := range joined {
		names[i] = joined[i].ServerName
	}

	edu := &gomatrixserverlib.EDU{Type: ote.Event.Type}
	if edu.Content, err = json.Marshal(map[string]interface{}{
		"room_id": ote.Event.RoomID,
		"user_id": ote.Event.UserID,
		"typing":  ote.Event.Typing,
	}); err != nil {
		return err
	}

	return t.queues.SendEDU(edu, t.ServerName, names)
}
