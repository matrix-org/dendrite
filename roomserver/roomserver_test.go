package roomserver

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

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

type fledglingEvent struct {
	Type     string
	StateKey *string
	Content  interface{}
	Sender   string
	RoomID   string
}

func mustCreateEvents(t *testing.T, roomVer gomatrixserverlib.RoomVersion, events []fledglingEvent) (result []gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	depth := int64(1)
	seed := make([]byte, ed25519.SeedSize) // zero seed
	key := ed25519.NewKeyFromSeed(seed)
	var prevs []string
	roomState := make(map[gomatrixserverlib.StateKeyTuple]string) // state -> event ID
	for _, ev := range events {
		eb := gomatrixserverlib.EventBuilder{
			Sender:     ev.Sender,
			Depth:      depth,
			Type:       ev.Type,
			StateKey:   ev.StateKey,
			RoomID:     ev.RoomID,
			PrevEvents: prevs,
		}
		err := eb.SetContent(ev.Content)
		if err != nil {
			t.Fatalf("mustCreateEvent: failed to marshal event content %+v", ev.Content)
		}
		stateNeeded, err := gomatrixserverlib.StateNeededForEventBuilder(&eb)
		if err != nil {
			t.Fatalf("mustCreateEvent: failed to work out auth_events : %s", err)
		}
		var authEvents []string
		for _, tuple := range stateNeeded.Tuples() {
			eventID := roomState[tuple]
			if eventID != "" {
				authEvents = append(authEvents, eventID)
			}
		}
		eb.AuthEvents = authEvents
		signedEvent, err := eb.Build(time.Now(), testOrigin, "ed25519:test", key, roomVer)
		if err != nil {
			t.Fatalf("mustCreateEvent: failed to sign event: %s", err)
		}
		depth++
		prevs = []string{signedEvent.EventID()}
		if ev.StateKey != nil {
			roomState[gomatrixserverlib.StateKeyTuple{
				EventType: ev.Type,
				StateKey:  *ev.StateKey,
			}] = signedEvent.EventID()
		}
		result = append(result, signedEvent.Headered(roomVer))
	}
	return
}

func mustLoadRawEvents(t *testing.T, ver gomatrixserverlib.RoomVersion, events []json.RawMessage) []gomatrixserverlib.HeaderedEvent {
	t.Helper()
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

func mustCreateRoomserverAPI(t *testing.T) (api.RoomserverInternalAPI, *dummyProducer) {
	t.Helper()
	cfg := &config.Dendrite{}
	cfg.Defaults()
	cfg.Global.ServerName = testOrigin
	cfg.Global.Kafka.UseNaffka = true
	cfg.RoomServer.Database.ConnectionString = config.DataSource(roomserverDBFileURI)
	dp := &dummyProducer{
		topic: cfg.Global.Kafka.TopicFor(config.TopicOutputRoomEvent),
	}
	cache, err := caching.NewInMemoryLRUCache(false)
	if err != nil {
		t.Fatalf("failed to make caches: %s", err)
	}
	base := &setup.BaseDendrite{
		KafkaProducer: dp,
		Caches:        cache,
		Cfg:           cfg,
	}

	return NewInternalAPI(base, &test.NopJSONVerifier{}), dp
}

func mustSendEvents(t *testing.T, ver gomatrixserverlib.RoomVersion, events []json.RawMessage) (api.RoomserverInternalAPI, *dummyProducer, []gomatrixserverlib.HeaderedEvent) {
	t.Helper()
	rsAPI, dp := mustCreateRoomserverAPI(t)
	hevents := mustLoadRawEvents(t, ver, events)
	if err := api.SendEvents(ctx, rsAPI, hevents, testOrigin, nil); err != nil {
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

// This tests that rewriting state works correctly.
// This creates a small room with a create/join/name state, then replays it
// with a new room name. We expect the output events to contain the original events,
// followed by a single OutputNewRoomEvent with RewritesState set to true with the
// rewritten state events (with the 2nd room name).
func TestOutputRewritesState(t *testing.T) {
	roomID := "!foo:" + string(testOrigin)
	alice := "@alice:" + string(testOrigin)
	emptyKey := ""
	originalEvents := mustCreateEvents(t, gomatrixserverlib.RoomVersionV6, []fledglingEvent{
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"creator":      alice,
				"room_version": "6",
			},
			StateKey: &emptyKey,
			Type:     gomatrixserverlib.MRoomCreate,
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"membership": "join",
			},
			StateKey: &alice,
			Type:     gomatrixserverlib.MRoomMember,
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"body": "hello world",
			},
			StateKey: nil,
			Type:     "m.room.message",
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"name": "Room Name",
			},
			StateKey: &emptyKey,
			Type:     "m.room.name",
		},
	})
	rewriteEvents := mustCreateEvents(t, gomatrixserverlib.RoomVersionV6, []fledglingEvent{
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"creator": alice,
			},
			StateKey: &emptyKey,
			Type:     gomatrixserverlib.MRoomCreate,
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"membership": "join",
			},
			StateKey: &alice,
			Type:     gomatrixserverlib.MRoomMember,
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"name": "Room Name 2",
			},
			StateKey: &emptyKey,
			Type:     "m.room.name",
		},
		{
			RoomID: roomID,
			Sender: alice,
			Content: map[string]interface{}{
				"body": "hello world 2",
			},
			StateKey: nil,
			Type:     "m.room.message",
		},
	})
	deleteDatabase()
	rsAPI, producer := mustCreateRoomserverAPI(t)
	defer deleteDatabase()
	err := api.SendEvents(context.Background(), rsAPI, originalEvents, testOrigin, nil)
	if err != nil {
		t.Fatalf("failed to send original events: %s", err)
	}
	// assert we got them produced, this is just a sanity check and isn't the intention of this test
	if len(producer.producedMessages) != len(originalEvents) {
		t.Fatalf("SendEvents didn't result in same number of produced output events: got %d want %d", len(producer.producedMessages), len(originalEvents))
	}
	producer.producedMessages = nil // we aren't actually interested in these events, just the rewrite ones

	var inputEvents []api.InputRoomEvent
	// slowly build up the state IDs again, we're basically telling the roomserver what to store as a snapshot
	var stateIDs []string
	// skip the last event, we'll use this to tie together the rewrite as the KindNew event
	for i := 0; i < len(rewriteEvents)-1; i++ {
		ev := rewriteEvents[i]
		inputEvents = append(inputEvents, api.InputRoomEvent{
			Kind:          api.KindOutlier,
			Event:         ev,
			AuthEventIDs:  ev.AuthEventIDs(),
			HasState:      true,
			StateEventIDs: stateIDs,
		})
		if ev.StateKey() != nil {
			stateIDs = append(stateIDs, ev.EventID())
		}
	}
	lastEv := rewriteEvents[len(rewriteEvents)-1]
	inputEvents = append(inputEvents, api.InputRoomEvent{
		Kind:          api.KindNew,
		Event:         lastEv,
		AuthEventIDs:  lastEv.AuthEventIDs(),
		HasState:      true,
		StateEventIDs: stateIDs,
	})
	if err := api.SendInputRoomEvents(context.Background(), rsAPI, inputEvents); err != nil {
		t.Fatalf("SendInputRoomEvents returned error for rewrite events: %s", err)
	}
	// we should just have one output event with the entire state of the room in it
	if len(producer.producedMessages) != 1 {
		t.Fatalf("Rewritten events got output, want only 1 got %d", len(producer.producedMessages))
	}
	outputEvent := producer.producedMessages[0]
	if !outputEvent.NewRoomEvent.RewritesState {
		t.Errorf("RewritesState flag not set on output event")
	}
	if !reflect.DeepEqual(stateIDs, outputEvent.NewRoomEvent.AddsStateEventIDs) {
		t.Errorf("Output event is missing room state event IDs, got %v want %v", outputEvent.NewRoomEvent.AddsStateEventIDs, stateIDs)
	}
	if !bytes.Equal(outputEvent.NewRoomEvent.Event.JSON(), lastEv.JSON()) {
		t.Errorf(
			"Output event isn't the latest KindNew event:\ngot  %s\nwant %s",
			string(outputEvent.NewRoomEvent.Event.JSON()),
			string(lastEv.JSON()),
		)
	}
	if len(outputEvent.NewRoomEvent.AddStateEvents) != len(stateIDs) {
		t.Errorf("Output event is missing room state events themselves, got %d want %d", len(outputEvent.NewRoomEvent.AddStateEvents), len(stateIDs))
	}
	// make sure the state got overwritten, check the room name
	hasRoomName := false
	for _, ev := range outputEvent.NewRoomEvent.AddStateEvents {
		if ev.Type() == "m.room.name" {
			hasRoomName = string(ev.Content()) == `{"name":"Room Name 2"}`
		}
	}
	if !hasRoomName {
		t.Errorf("Output event did not overwrite room state")
	}
}
