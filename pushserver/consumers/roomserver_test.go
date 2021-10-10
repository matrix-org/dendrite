package consumers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/matrix-org/dendrite/internal/pushgateway"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/pushserver/api"
	"github.com/matrix-org/dendrite/pushserver/producers"
	"github.com/matrix-org/dendrite/pushserver/storage"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

const serverName = gomatrixserverlib.ServerName("example.org")

func TestOutputRoomEventConsumer(t *testing.T) {
	ctx := context.Background()

	dbopts := &config.DatabaseOptions{
		ConnectionString:   "file::memory:",
		MaxOpenConnections: 1,
		MaxIdleConnections: 1,
	}
	db, err := storage.Open(dbopts)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	err = db.CreatePusher(ctx,
		api.Pusher{
			PushKey: "apushkey",
			Kind:    api.HTTPKind,
			AppID:   "anappid",
			Data: map[string]interface{}{
				"url":   "http://example.org/pusher/notify",
				"extra": "someextra",
			},
		},
		"alice")
	if err != nil {
		t.Fatalf("CreatePusher failed: %v", err)
	}

	var rsAPI fakeRoomServerInternalAPI
	var psAPI fakePushserverInternalAPI
	var messageSender fakeMessageSender
	var wg sync.WaitGroup
	wg.Add(1)
	pgClient := fakePushGatewayClient{
		WG: &wg,
	}
	s := &OutputRoomEventConsumer{
		cfg: &config.PushServer{
			Matrix: &config.Global{
				ServerName: serverName,
			},
		},
		db:           db,
		rsAPI:        &rsAPI,
		psAPI:        &psAPI,
		pgClient:     &pgClient,
		syncProducer: producers.NewSyncAPI(db, &messageSender, "clientDataTopic", "notificationDataTopic"),
	}

	event, err := gomatrixserverlib.NewEventFromTrustedJSONWithEventID("$143273582443PhrSn:example.org", []byte(`{
  "content": {
    "body": "This is an example text message",
    "format": "org.matrix.custom.html",
    "formatted_body": "\u003cb\u003eThis is an example text message\u003c/b\u003e",
    "msgtype": "m.text"
  },
  "origin_server_ts": 1432735824653,
  "room_id": "!jEsUZKDJdhlrceRyVU:example.org",
  "sender": "@example:example.org",
  "type": "m.room.message",
  "unsigned": {
    "age": 1234
  }
}`), false, gomatrixserverlib.RoomVersionV7)
	if err != nil {
		t.Fatalf("NewEventFromTrustedJSON failed: %v", err)
	}

	ev := &gomatrixserverlib.HeaderedEvent{
		Event: event,
	}
	if err := s.processMessage(ctx, ev); err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}

	t.Log("Waiting for backend calls to finish.")
	wg.Wait()

	if diff := cmp.Diff([]*rsapi.QueryMembershipsForRoomRequest{{JoinedOnly: true, RoomID: "!jEsUZKDJdhlrceRyVU:example.org"}}, rsAPI.MembershipReqs); diff != "" {
		t.Errorf("rsAPI.QueryMembershipsForRoom Reqs: +got -want:\n%s", diff)
	}
	if diff := cmp.Diff([]*pushgateway.NotifyRequest{{
		Notification: pushgateway.Notification{
			Type:    "m.room.message",
			Content: event.Content(),
			Counts: &pushgateway.Counts{
				Unread: 1,
			},
			Devices: []*pushgateway.Device{{
				AppID:   "anappid",
				PushKey: "apushkey",
				Data: map[string]interface{}{
					"extra": "someextra",
				},
			}},
			EventID:  "$143273582443PhrSn:example.org",
			ID:       "$143273582443PhrSn:example.org",
			RoomID:   "!jEsUZKDJdhlrceRyVU:example.org",
			RoomName: "aname",
			Sender:   "@example:example.org",
		},
	}}, pgClient.Reqs); diff != "" {
		t.Errorf("pgClient.NotifyHTTP Reqs: +got -want:\n%s", diff)
	}
	if diff := cmp.Diff([]sarama.ProducerMessage{{
		Topic: "notificationDataTopic",
		Key:   sarama.StringEncoder("@alice:example.org"),
		Value: sarama.ByteEncoder([]byte(`{"room_id":"!jEsUZKDJdhlrceRyVU:example.org","unread_highlight_count":0,"unread_notification_count":1}`)),
	}}, messageSender.Messages, cmpopts.IgnoreUnexported(sarama.ProducerMessage{})); diff != "" {
		t.Errorf("SendMessage Messages: +got -want:\n%s", diff)
	}
}

type fakeRoomServerInternalAPI struct {
	rsapi.RoomserverInternalAPI

	MembershipReqs []*rsapi.QueryMembershipsForRoomRequest
}

func (s *fakeRoomServerInternalAPI) QueryCurrentState(
	ctx context.Context,
	req *rsapi.QueryCurrentStateRequest,
	res *rsapi.QueryCurrentStateResponse,
) error {
	*res = rsapi.QueryCurrentStateResponse{
		StateEvents: map[gomatrixserverlib.StateKeyTuple]*gomatrixserverlib.HeaderedEvent{
			roomNameTuple: mustParseHeaderedEventJSON(`{
  "_room_version": "7",
  "content": {
    "name": "aname"
  },
  "event_id": "$3957tyerfgewrf382:example.org",
  "origin_server_ts": 1432735824652,
  "room_id": "!jEsUZKDJdhlrceRyVU:example.org",
  "sender": "@example:example.org",
  "state_key": "@alice:example.org",
  "type": "m.room.name"
}`),
		},
	}
	return nil
}

func (s *fakeRoomServerInternalAPI) QueryMembershipsForRoom(
	ctx context.Context,
	req *rsapi.QueryMembershipsForRoomRequest,
	res *rsapi.QueryMembershipsForRoomResponse,
) error {
	s.MembershipReqs = append(s.MembershipReqs, req)
	*res = rsapi.QueryMembershipsForRoomResponse{
		JoinEvents: []gomatrixserverlib.ClientEvent{
			mustParseClientEventJSON(`{
  "content": {
    "avatar_url": "mxc://example.org/SEsfnsuifSDFSSEF",
    "displayname": "Alice Margatroid",
    "membership": "join",
    "reason": "Looking for support"
  },
  "event_id": "$3957tyerfgewrf384:example.org",
  "origin_server_ts": 1432735824653,
  "room_id": "!jEsUZKDJdhlrceRyVU:example.org",
  "sender": "@example:example.org",
  "state_key": "@alice:example.org",
  "type": "m.room.member",
  "unsigned": {
    "age": 1234
  }
}`),
		},
	}
	return nil
}

type fakePushserverInternalAPI struct {
	api.PushserverInternalAPI
}

func (s *fakePushserverInternalAPI) QueryPushRules(ctx context.Context, req *api.QueryPushRulesRequest, res *api.QueryPushRulesResponse) error {
	localpart, _, err := gomatrixserverlib.SplitID('@', req.UserID)
	if err != nil {
		return err
	}
	res.RuleSets = pushrules.DefaultAccountRuleSets(localpart, "example.org")
	return nil
}

type fakePushGatewayClient struct {
	pushgateway.Client

	WG   *sync.WaitGroup
	Reqs []*pushgateway.NotifyRequest
}

func (c *fakePushGatewayClient) Notify(ctx context.Context, url string, req *pushgateway.NotifyRequest, res *pushgateway.NotifyResponse) error {
	c.Reqs = append(c.Reqs, req)
	if c.WG != nil {
		c.WG.Done()
	}
	*res = pushgateway.NotifyResponse{
		Rejected: []string{
			"apushkey",
		},
	}
	return nil
}

func mustMarshalJSON(v interface{}) []byte {
	bs, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bs
}

func mustParseClientEventJSON(s string) gomatrixserverlib.ClientEvent {
	var ev gomatrixserverlib.ClientEvent
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		panic(err)
	}
	return ev
}

func mustParseHeaderedEventJSON(s string) *gomatrixserverlib.HeaderedEvent {
	var ev gomatrixserverlib.HeaderedEvent
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		panic(err)
	}
	return &ev
}

type fakeMessageSender struct {
	Messages []sarama.ProducerMessage
}

func (s *fakeMessageSender) SendMessage(msg *sarama.ProducerMessage) (partition int32, offset int64, err error) {
	s.Messages = append(s.Messages, *msg)
	return 0, 0, nil
}
