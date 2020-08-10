package roomserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	testOrigin = gomatrixserverlib.ServerName("kaer.morhen")
	// we have to use an on-disk DB because we open multiple connections due to the *Updater structs.
	// Using :memory: results in a brand new DB for each open connection, and sharing memory via
	// ?cache=shared just allows read-only sharing, so writes to the database on other connections are lost.
	roomserverDBFileURI  = "file:roomserver_test.db"
	roomserverDBFilePath = "./roomserver_test.db"
)

var (
	ctx = context.Background()
)

type dummyProducer struct {
	topic            string
	producedMessages []*api.OutputEvent
}

// SendMessage produces a given message, and returns only when it either has
// succeeded or failed to produce. It will return the partition and the offset
// of the produced message, or an error if the message failed to produce.
func (p *dummyProducer) SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	if msg.Topic != p.topic {
		return 0, 0, nil
	}
	be := msg.Value.(sarama.ByteEncoder)
	b := json.RawMessage(be)
	fmt.Println("SENDING >>>>>>>> ", string(b))
	var out api.OutputEvent
	err = json.Unmarshal(b, &out)
	if err != nil {
		return 0, 0, err
	}
	p.producedMessages = append(p.producedMessages, &out)
	return 0, 0, nil
}

// SendMessages produces a given set of messages, and returns only when all
// messages in the set have either succeeded or failed. Note that messages
// can succeed and fail individually; if some succeed and some fail,
// SendMessages will return an error.
func (p *dummyProducer) SendMessages(msgs []*sarama.ProducerMessage) error {
	for _, m := range msgs {
		p.SendMessage(m)
	}
	return nil
}

// Close shuts down the producer and waits for any buffered messages to be
// flushed. You must call this function before a producer object passes out of
// scope, as it may otherwise leak memory. You must call this before calling
// Close on the underlying client.
func (p *dummyProducer) Close() error {
	return nil
}

func deleteDatabase() {
	err := os.Remove(roomserverDBFilePath)
	if err != nil {
		fmt.Printf("failed to delete database %s: %s\n", roomserverDBFilePath, err)
	}
}

func mustLoadEvents(t *testing.T, ver gomatrixserverlib.RoomVersion, events []json.RawMessage) []gomatrixserverlib.HeaderedEvent {
	hs := make([]gomatrixserverlib.HeaderedEvent, len(events))
	for i := range events {
		e, err := gomatrixserverlib.NewEventFromTrustedJSON(events[i], false, ver)
		if err != nil {
			t.Fatalf("cannot load test data: " + err.Error())
		}
		h := e.Headered(ver)
		hs[i] = h
	}
	return hs
}

func mustSendEvents(t *testing.T, ver gomatrixserverlib.RoomVersion, events []json.RawMessage) (api.RoomserverInternalAPI, *dummyProducer, []gomatrixserverlib.HeaderedEvent) {
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = testOrigin
	cfg.Global.Kafka.UseNaffka = true
	cfg.RoomServer.Database.ConnectionString = config.DataSource(roomserverDBFileURI)
	dp := &dummyProducer{
		topic: cfg.Global.Kafka.TopicFor(config.TopicOutputRoomEvent),
	}
	cache, err := caching.NewInMemoryLRUCache(true)
	if err != nil {
		t.Fatalf("failed to make caches: %s", err)
	}
	base := &setup.BaseDendrite{
		KafkaProducer: dp,
		Caches:        cache,
		Cfg:           cfg,
	}

	rsAPI := NewInternalAPI(base, &test.NopJSONVerifier{}, nil)
	hevents := mustLoadEvents(t, ver, events)
	_, err = api.SendEvents(ctx, rsAPI, hevents, testOrigin, nil)
	if err != nil {
		t.Errorf("failed to SendEvents: %s", err)
	}
	return rsAPI, dp, hevents
}

func TestOutputRedactedEvent(t *testing.T) {
	redactionEvents := []json.RawMessage{
		// create event
		[]byte(`{"auth_events":[],"content":{"creator":"@userid:kaer.morhen"},"depth":0,"event_id":"$N4us6vqqq3RjvpKd:kaer.morhen","hashes":{"sha256":"WTdrCn/YsiounXcJPsLP8xT0ZjHiO5Ov0NvXYmK2onE"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"9+5JcpaN5b5KlHYHGp6r+GoNDH98lbfzGYwjfxensa5C5D/bDACaYnMDLnhwsHOE5nxgI+jT/GV271pz6PMSBQ"}},"state_key":"","type":"m.room.create"}`),
		// join event
		[]byte(`{"auth_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}]],"content":{"membership":"join"},"depth":1,"event_id":"$6sUiGPQ0a3tqYGKo:kaer.morhen","hashes":{"sha256":"eYVBC7RO+FlxRyW1aXYf/ad4Dzi7T93tArdGw3r4RwQ"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"tiDBTPFa53YMfHiupX3vSRE/ZcCiCjmGt7gDpIpDpwZapeays5Vqqcqb7KiywrDldpTkrrdJBAw2jXcq6ZyhDw"}},"state_key":"@userid:kaer.morhen","type":"m.room.member"}`),
		// room name
		[]byte(`{"auth_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}],["$6sUiGPQ0a3tqYGKo:kaer.morhen",{"sha256":"IS4HSMqpqVUGh1Z3qgC99YcaizjCoO4yFhYYe8j53IE"}]],"content":{"name":"My Room Name"},"depth":2,"event_id":"$VC1zZ9YWwuUbSNHD:kaer.morhen","hashes":{"sha256":"bpqTkfLx6KHzWz7/wwpsXnXwJWEGW14aV63ffexzDFg"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$6sUiGPQ0a3tqYGKo:kaer.morhen",{"sha256":"IS4HSMqpqVUGh1Z3qgC99YcaizjCoO4yFhYYe8j53IE"}]],"prev_state":[],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"mhJZ3X4bAKrF/T0mtPf1K2Tmls0h6xGY1IPDpJ/SScQBqDlu3HQR2BPa7emqj5bViyLTWVNh+ZCpzx/6STTrAg"}},"state_key":"","type":"m.room.name"}`),
		// redact room name
		[]byte(`{"auth_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}],["$6sUiGPQ0a3tqYGKo:kaer.morhen",{"sha256":"IS4HSMqpqVUGh1Z3qgC99YcaizjCoO4yFhYYe8j53IE"}]],"content":{"reason":"Spamming"},"depth":3,"event_id":"$tJI0pE3b8u9UMYpT:kaer.morhen","hashes":{"sha256":"/3TStqa5SQqYaEtl7ajEvSRvu6d12MMKfICUzrBpd2Q"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$VC1zZ9YWwuUbSNHD:kaer.morhen",{"sha256":"+l8cNa7syvm0EF7CAmQRlYknLEMjivnI4FLhB/TUBEY"}]],"redacts":"$VC1zZ9YWwuUbSNHD:kaer.morhen","room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"QBOh+amf0vTJbm6+9VwAcR9uJviBIor2KON0Y7+EyQx5YbUZEzW1HPeJxarLIHBcxMzgOVzjuM+StzjbUgDzAg"}},"type":"m.room.redaction"}`),
		// message
		[]byte(`{"auth_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}],["$6sUiGPQ0a3tqYGKo:kaer.morhen",{"sha256":"IS4HSMqpqVUGh1Z3qgC99YcaizjCoO4yFhYYe8j53IE"}]],"content":{"body":"Test Message"},"depth":4,"event_id":"$o8KHsgSIYbJrddnd:kaer.morhen","hashes":{"sha256":"IE/rGVlKOpiGWeIo887g1CK1drYqcWDZhL6THZHkJ1c"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$tJI0pE3b8u9UMYpT:kaer.morhen",{"sha256":"zvmwyXuDox7jpA16JRH6Fc1zbfQht2tpkBbMTUOi3Jw"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"/3z+pJjiJXWhwfqIEzmNksvBHCoXTktK/y0rRuWJXw6i1+ygRG/suDCKhFuuz6gPapRmEMPVILi2mJqHHXPKAg"}},"type":"m.room.message"}`),
		// redact previous message
		[]byte(`{"auth_events":[["$N4us6vqqq3RjvpKd:kaer.morhen",{"sha256":"SylirfgfXFhscZL7p10NmOa1nFFEckiwz0lAideQMIM"}],["$6sUiGPQ0a3tqYGKo:kaer.morhen",{"sha256":"IS4HSMqpqVUGh1Z3qgC99YcaizjCoO4yFhYYe8j53IE"}]],"content":{"reason":"Spamming more"},"depth":5,"event_id":"$UpsE8belb2gJItJG:kaer.morhen","hashes":{"sha256":"zU8PWJOld/I7OtjdpltFSKC+DMNm2ZyEXAHcprsafD0"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$o8KHsgSIYbJrddnd:kaer.morhen",{"sha256":"UgjMuCFXH4warIjKuwlRq9zZ6dSJrZWCd+CkqtgLSHM"}]],"redacts":"$o8KHsgSIYbJrddnd:kaer.morhen","room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"zxFGr/7aGOzqOEN6zRNrBpFkkMnfGFPbCteYL33wC+PycBPIK+2WRa5qlAR2+lcLiK3HjIzwRYkKNsVFTqvRAw"}},"type":"m.room.redaction"}`),
	}
	var redactedOutputs []api.OutputEvent
	deleteDatabase()
	_, producer, hevents := mustSendEvents(t, gomatrixserverlib.RoomVersionV1, redactionEvents)
	defer deleteDatabase()
	for _, msg := range producer.producedMessages {
		if msg.Type == api.OutputTypeRedactedEvent {
			redactedOutputs = append(redactedOutputs, *msg)
		}
	}
	wantRedactedOutputs := []api.OutputEvent{
		{
			Type: api.OutputTypeRedactedEvent,
			RedactedEvent: &api.OutputRedactedEvent{
				RedactedEventID: hevents[2].EventID(),
				RedactedBecause: hevents[3],
			},
		},
		{
			Type: api.OutputTypeRedactedEvent,
			RedactedEvent: &api.OutputRedactedEvent{
				RedactedEventID: hevents[4].EventID(),
				RedactedBecause: hevents[5],
			},
		},
	}
	t.Logf("redactedOutputs: %+v", redactedOutputs)
	if len(wantRedactedOutputs) != len(redactedOutputs) {
		t.Fatalf("Got %d redacted events, want %d", len(redactedOutputs), len(wantRedactedOutputs))
	}
	for i := 0; i < len(wantRedactedOutputs); i++ {
		if !reflect.DeepEqual(*redactedOutputs[i].RedactedEvent, *wantRedactedOutputs[i].RedactedEvent) {
			t.Errorf("OutputRedactionEvent %d: wrong event got:\n%+v want:\n%+v", i+1, redactedOutputs[i].RedactedEvent, wantRedactedOutputs[i].RedactedEvent)
		}
	}
}
