package consumers

import (
	"context"
	"encoding/json"

	eduapi "github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"

	"github.com/Shopify/sarama"
)

type OutputCrossSigningKeyUpdateConsumer struct {
	eduServerConsumer *internal.ContinualConsumer
	keyDB             storage.Database
	keyAPI            api.KeyInternalAPI
	serverName        string
}

func NewOutputCrossSigningKeyUpdateConsumer(
	process *process.ProcessContext,
	cfg *config.Dendrite,
	kafkaConsumer sarama.Consumer,
	keyDB storage.Database,
	keyAPI api.KeyInternalAPI,
) *OutputCrossSigningKeyUpdateConsumer {
	consumer := internal.ContinualConsumer{
		Process:        process,
		ComponentName:  "keyserver/eduserver",
		Topic:          cfg.Global.Kafka.TopicFor(config.TopicOutputCrossSigningKeyUpdate),
		Consumer:       kafkaConsumer,
		PartitionStore: keyDB,
	}
	s := &OutputCrossSigningKeyUpdateConsumer{
		eduServerConsumer: &consumer,
		keyDB:             keyDB,
		keyAPI:            keyAPI,
		serverName:        string(cfg.Global.ServerName),
	}
	consumer.ProcessMessage = s.onMessage

	return s
}

func (s *OutputCrossSigningKeyUpdateConsumer) Start() error {
	return s.eduServerConsumer.Start()
}

func (s *OutputCrossSigningKeyUpdateConsumer) onMessage(msg *sarama.ConsumerMessage) error {
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
	if host == gomatrixserverlib.ServerName(s.serverName) {
		// Ignore any messages that contain information about our own users, as
		// they already originated from this server.
		return nil
	}
	uploadReq := &api.PerformUploadDeviceKeysRequest{
		UserID: output.UserID,
	}
	if output.MasterKey != nil {
		uploadReq.MasterKey = *output.MasterKey
	}
	if output.SelfSigningKey != nil {
		uploadReq.SelfSigningKey = *output.SelfSigningKey
	}
	uploadRes := &api.PerformUploadDeviceKeysResponse{}
	s.keyAPI.PerformUploadDeviceKeys(context.TODO(), uploadReq, uploadRes)
	return uploadRes.Error
}
