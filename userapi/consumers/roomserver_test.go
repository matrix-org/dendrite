package consumers

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"

	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi/storage"
	userAPITypes "github.com/matrix-org/dendrite/userapi/types"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := storage.NewUserAPIDatabase(nil, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, "", 4, 0, 0, "")
	if err != nil {
		t.Fatalf("failed to create new user db: %v", err)
	}
	return db, close
}

func mustCreateEvent(t *testing.T, content string) *gomatrixserverlib.HeaderedEvent {
	t.Helper()
	ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(content), false, gomatrixserverlib.RoomVersionV10)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	return ev.Headered(gomatrixserverlib.RoomVersionV10)
}

func Test_evaluatePushRules(t *testing.T) {
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		consumer := OutputRoomEventConsumer{db: db}

		testCases := []struct {
			name         string
			eventContent string
			wantAction   pushrules.ActionKind
			wantActions  []*pushrules.Action
			wantNotify   bool
		}{
			{
				name:         "m.receipt doesn't notify",
				eventContent: `{"type":"m.receipt"}`,
				wantAction:   pushrules.UnknownAction,
				wantActions:  nil,
			},
			{
				name:         "m.reaction doesn't notify",
				eventContent: `{"type":"m.reaction"}`,
				wantAction:   pushrules.DontNotifyAction,
				wantActions: []*pushrules.Action{
					{
						Kind: pushrules.DontNotifyAction,
					},
				},
			},
			{
				name:         "m.room.message notifies",
				eventContent: `{"type":"m.room.message"}`,
				wantNotify:   true,
				wantAction:   pushrules.NotifyAction,
				wantActions: []*pushrules.Action{
					{Kind: pushrules.NotifyAction},
					{
						Kind:  pushrules.SetTweakAction,
						Tweak: pushrules.HighlightTweak,
						Value: false,
					},
				},
			},
			{
				name:         "m.room.message highlights",
				eventContent: `{"type":"m.room.message", "content": {"body": "test"} }`,
				wantNotify:   true,
				wantAction:   pushrules.NotifyAction,
				wantActions: []*pushrules.Action{
					{Kind: pushrules.NotifyAction},
					{
						Kind:  pushrules.SetTweakAction,
						Tweak: pushrules.SoundTweak,
						Value: "default",
					},
					{
						Kind:  pushrules.SetTweakAction,
						Tweak: pushrules.HighlightTweak,
						Value: true,
					},
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				actions, err := consumer.evaluatePushRules(ctx, mustCreateEvent(t, tc.eventContent), &localMembership{
					UserID:    "@test:localhost",
					Localpart: "test",
					Domain:    "localhost",
				}, 10)
				if err != nil {
					t.Fatalf("failed to evaluate push rules: %v", err)
				}
				assert.Equal(t, tc.wantActions, actions)
				gotAction, _, err := pushrules.ActionsToTweaks(actions)
				if err != nil {
					t.Fatalf("failed to get actions: %v", err)
				}
				if gotAction != tc.wantAction {
					t.Fatalf("expected action to be '%s', got '%s'", tc.wantAction, gotAction)
				}
				// this is taken from `notifyLocal`
				if tc.wantNotify && gotAction != pushrules.NotifyAction && gotAction != pushrules.CoalesceAction {
					t.Fatalf("expected to notify but didn't")
				}
			})

		}
	})
}

func TestMessageStats(t *testing.T) {
	type args struct {
		eventType   string
		eventSender string
	}
	tests := []struct {
		name      string
		args      args
		ourServer gomatrixserverlib.ServerName
		wantStats userAPITypes.MessageStats
	}{
		{
			name:      "m.room.create does not count as a message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.create",
				eventSender: "@alice:localhost",
			},
		},
		{
			name:      "our server - message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.message",
				eventSender: "@alice:localhost",
			},
			wantStats: userAPITypes.MessageStats{Messages: 1, SentMessages: 1},
		},
		{
			name:      "our server - E2EE message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.encrypted",
				eventSender: "@alice:localhost",
			},
			wantStats: userAPITypes.MessageStats{Messages: 1, SentMessages: 1, MessagesE2EE: 1, SentMessagesE2EE: 1},
		},

		{
			name:      "remote server - message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.message",
				eventSender: "@alice:remote",
			},
			wantStats: userAPITypes.MessageStats{Messages: 2, SentMessages: 1, MessagesE2EE: 1, SentMessagesE2EE: 1},
		},
		{
			name:      "remote server - E2EE message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.encrypted",
				eventSender: "@alice:remote",
			},
			wantStats: userAPITypes.MessageStats{Messages: 2, SentMessages: 1, MessagesE2EE: 2, SentMessagesE2EE: 1},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				s := &OutputRoomEventConsumer{
					db:            db,
					msgCounts:     map[gomatrixserverlib.ServerName]userAPITypes.MessageStats{},
					msgCountsLock: sync.Mutex{},
					serverName:    tt.ourServer,
				}
				s.storeMessageStats(context.Background(), tt.args.eventType, tt.args.eventSender)

				gotStats, err := db.DailyMessages(context.Background(), tt.ourServer)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if !reflect.DeepEqual(gotStats, tt.wantStats) {
					t.Fatalf("expected %+v, got %+v", tt.wantStats, gotStats)
				}
			})
		}
	})
}
