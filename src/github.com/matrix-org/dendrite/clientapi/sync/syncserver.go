package sync

import (
	"encoding/json"

	log "github.com/Sirupsen/logrus"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/storage"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// syncStreamPosition represents the offset in the sync stream a client is at.
type syncStreamPosition int64

// Server contains all the logic for running a sync server
type Server struct {
	roomServerConsumer *common.ContinualConsumer
	db                 *storage.SyncServerDatabase
	rp                 *RequestPool
}

// NewServer creates a new sync server. Call Start() to begin consuming from room servers.
func NewServer(cfg *config.Sync, rp *RequestPool, store *storage.SyncServerDatabase) (*Server, error) {
	kafkaConsumer, err := sarama.NewConsumer(cfg.KafkaConsumerURIs, nil)
	if err != nil {
		return nil, err
	}

	consumer := common.ContinualConsumer{
		Topic:          cfg.RoomserverOutputTopic,
		Consumer:       kafkaConsumer,
		PartitionStore: store,
	}
	s := &Server{
		roomServerConsumer: &consumer,
		db:                 store,
		rp:                 rp,
	}
	consumer.ProcessMessage = s.onMessage

	return s, nil
}

// Start consuming from room servers
func (s *Server) Start() error {
	return s.roomServerConsumer.Start()
}

// onMessage is called when the sync server receives a new event from the room server output log.
// It is not safe for this function to be called from multiple goroutines, or else the
// sync stream position may race and be incorrectly calculated.
func (s *Server) onMessage(msg *sarama.ConsumerMessage) error {
	// Parse out the event JSON
	var output api.OutputRoomEvent
	if err := json.Unmarshal(msg.Value, &output); err != nil {
		// If the message was invalid, log it and move on to the next message in the stream
		log.WithError(err).Errorf("roomserver output log: message parse failure")
		return nil
	}

	ev, err := gomatrixserverlib.NewEventFromTrustedJSON(output.Event, false)
	if err != nil {
		log.WithError(err).Errorf("roomserver output log: event parse failure")
		return nil
	}
	log.WithFields(log.Fields{
		"event_id": ev.EventID(),
		"room_id":  ev.RoomID(),
	}).Info("received event from roomserver")

	syncStreamPos, err := s.db.WriteEvent(&ev, output.AddsStateEventIDs, output.RemovesStateEventIDs)

	if err != nil {
		// panic rather than continue with an inconsistent database
		log.WithFields(log.Fields{
			"event":      string(ev.JSON()),
			log.ErrorKey: err,
			"add":        output.AddsStateEventIDs,
			"del":        output.RemovesStateEventIDs,
		}).Panicf("roomserver output log: write event failure")
		return nil
	}
	s.rp.OnNewEvent(&ev, syncStreamPosition(syncStreamPos))

	return nil
}
