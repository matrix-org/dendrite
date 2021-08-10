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
	"sync"

	"github.com/Shopify/sarama"
	"github.com/getsentry/sentry-go"
	eduserverAPI "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	roomserverAPI "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/syncapi/notifier"
	"github.com/matrix-org/dendrite/syncapi/storage"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

// OutputCrossSigningKeyUpdateConsumer consumes events that originated in the key server.
type OutputCrossSigningKeyUpdateConsumer struct {
	keyChangeConsumer   *internal.ContinualConsumer
	db                  storage.Database
	notifier            *notifier.Notifier
	stream              types.PartitionedStreamProvider
	serverName          gomatrixserverlib.ServerName // our server name
	rsAPI               roomserverAPI.RoomserverInternalAPI
	keyAPI              api.KeyInternalAPI
	partitionToOffset   map[int32]int64
	partitionToOffsetMu sync.Mutex
}

// NewOutputCrossSigningKeyUpdateConsumer creates a new OutputCrossSigningKeyUpdateConsumer.
// Call Start() to begin consuming from the key server.
func NewOutputCrossSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	serverName gomatrixserverlib.ServerName,
	topic string,
	kafkaConsumer sarama.Consumer,
	keyAPI api.KeyInternalAPI,
	rsAPI roomserverAPI.RoomserverInternalAPI,
	store storage.Database,
	notifier *notifier.Notifier,
	stream types.PartitionedStreamProvider,
) *OutputCrossSigningKeyUpdateConsumer {

	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "syncapi/crosssigning",
		Topic:          topic,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}

	s := &OutputCrossSigningKeyUpdateConsumer{
		keyChangeConsumer:   &consumer,
		db:                  store,
		serverName:          serverName,
		keyAPI:              keyAPI,
		rsAPI:               rsAPI,
		partitionToOffset:   make(map[int32]int64),
		partitionToOffsetMu: sync.Mutex{},
		notifier:            notifier,
		stream:              stream,
	}

	consumer.ProcessMessage = s.onMessage

	return s
}

// Start consuming from the key server
func (s *OutputCrossSigningKeyUpdateConsumer) Start() error {
	offsets, err := s.keyChangeConsumer.StartOffsets()
	s.partitionToOffsetMu.Lock()
	for _, o := range offsets {
		s.partitionToOffset[o.Partition] = o.Offset
	}
	s.partitionToOffsetMu.Unlock()
	return err
}

func (s *OutputCrossSigningKeyUpdateConsumer) updateOffset(msg *sarama.ConsumerMessage) {
	s.partitionToOffsetMu.Lock()
	defer s.partitionToOffsetMu.Unlock()
	s.partitionToOffset[msg.Partition] = msg.Offset
}

func (s *OutputCrossSigningKeyUpdateConsumer) onMessage(msg *sarama.ConsumerMessage) error {
	defer s.updateOffset(msg)

	var output eduserverAPI.OutputCrossSigningKeyUpdate
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		logrus.WithError(err).Errorf("eduserver output log: message parse failure")
		return nil
	}

	logrus.Infof("XXX: Sync API consumed crosssigning key update for %s", output.UserID)

	// work out who we need to notify about the new key
	var queryRes roomserverAPI.QuerySharedUsersResponse
	err := s.rsAPI.QuerySharedUsers(context.Background(), &roomserverAPI.QuerySharedUsersRequest{
		UserID: output.UserID,
	}, &queryRes)
	if err != nil {
		log.WithError(err).Error("syncapi: failed to QuerySharedUsers for key change event from key server")
		sentry.CaptureException(err)
		return err
	}
	// make sure we get our own key updates too!
	queryRes.UserIDsToCount[output.UserID] = 1
	posUpdate := types.LogPosition{
		Offset:    msg.Offset,
		Partition: msg.Partition,
	}

	s.stream.Advance(posUpdate)
	for userID := range queryRes.UserIDsToCount {
		s.notifier.OnNewKeyChange(types.StreamingToken{DeviceListPosition: posUpdate}, userID, output.UserID)
	}

	return nil
}
