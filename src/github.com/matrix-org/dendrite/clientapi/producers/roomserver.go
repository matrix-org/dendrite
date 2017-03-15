package producers

import (
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
	sarama "gopkg.in/Shopify/sarama.v1"
)

// RoomserverProducer produces events for the roomserver to consume.
type RoomserverProducer struct {
	Topic    string
	Producer sarama.SyncProducer
}

// NewRoomserverProducer creates a new RoomserverProducer
func NewRoomserverProducer(kafkaURIs []string, topic string) (*RoomserverProducer, error) {
	producer, err := sarama.NewSyncProducer(kafkaURIs, nil)
	if err != nil {
		return nil, err
	}
	return &RoomserverProducer{
		Topic:    topic,
		Producer: producer,
	}, nil
}

// SendEvents writes the given events to the roomserver input log. The events are written with KindNew.
func (c *RoomserverProducer) SendEvents(events []gomatrixserverlib.Event) error {
	eventIDs := make([]string, len(events))
	ires := make([]api.InputRoomEvent, len(events))
	for i := range events {
		var authEventIDs []string
		for _, ref := range events[i].AuthEvents() {
			authEventIDs = append(authEventIDs, ref.EventID)
		}
		ire := api.InputRoomEvent{
			Kind:         api.KindNew,
			Event:        events[i].JSON(),
			AuthEventIDs: authEventIDs,
		}
		ires[i] = ire
		eventIDs[i] = events[i].EventID()
	}
	return c.SendInputRoomEvents(ires, eventIDs)
}

// SendInputRoomEvents writes the given input room events to the roomserver input log. The length of both
// arrays must match, and each element must correspond to the same event.
func (c *RoomserverProducer) SendInputRoomEvents(ires []api.InputRoomEvent, eventIDs []string) error {
	// TODO: Nicer way of doing this. Options are:
	// A) Like this
	// B) Add EventID field to InputRoomEvent
	// C) Add wrapper struct with the EventID and the InputRoomEvent
	if len(eventIDs) != len(ires) {
		return fmt.Errorf("WriteInputRoomEvents: length mismatch %d != %d", len(eventIDs), len(ires))
	}

	msgs := make([]*sarama.ProducerMessage, len(ires))
	for i := range ires {
		msg, err := c.toProducerMessage(ires[i], eventIDs[i])
		if err != nil {
			return err
		}
		msgs[i] = msg
	}
	return c.Producer.SendMessages(msgs)
}

func (c *RoomserverProducer) toProducerMessage(ire api.InputRoomEvent, eventID string) (*sarama.ProducerMessage, error) {
	value, err := json.Marshal(ire)
	if err != nil {
		return nil, err
	}
	var m sarama.ProducerMessage
	m.Topic = c.Topic
	m.Key = sarama.StringEncoder(eventID)
	m.Value = sarama.ByteEncoder(value)
	return &m, nil
}
