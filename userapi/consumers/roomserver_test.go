package consumers

import (
	"context"
	"crypto/ed25519"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/internal/pushrules"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi/storage"
	userAPITypes "github.com/matrix-org/dendrite/userapi/types"
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.UserDatabase, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.NewUserDatabase(context.Background(), cm, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, "", 4, 0, 0, "")
	if err != nil {
		t.Fatalf("failed to create new user db: %v", err)
	}
	return db, func() {
		close()
	}
}

func mustCreateEvent(t *testing.T, content string) *types.HeaderedEvent {
	t.Helper()
	ev, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV10).NewEventFromTrustedJSON([]byte(content), false)
	if err != nil {
		t.Fatalf("failed to create event: %v", err)
	}
	return &types.HeaderedEvent{PDU: ev}
}

type FakeUserRoomserverAPI struct{ rsapi.UserRoomserverAPI }

func (f *FakeUserRoomserverAPI) QueryUserIDForSender(ctx context.Context, roomID spec.RoomID, senderID spec.SenderID) (*spec.UserID, error) {
	return spec.NewUserID(string(senderID), true)
}

func Test_evaluatePushRules(t *testing.T) {
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		consumer := OutputRoomEventConsumer{db: db, rsAPI: &FakeUserRoomserverAPI{}}

		testCases := []struct {
			name         string
			eventContent string
			wantAction   pushrules.ActionKind
			wantActions  []*pushrules.Action
			wantNotify   bool
		}{
			{
				name:         "m.receipt doesn't notify",
				eventContent: `{"type":"m.receipt","room_id":"!room:example.com"}`,
				wantAction:   pushrules.UnknownAction,
				wantActions:  nil,
			},
			{
				name:         "m.reaction doesn't notify",
				eventContent: `{"type":"m.reaction","room_id":"!room:example.com"}`,
				wantAction:   pushrules.DontNotifyAction,
				wantActions: []*pushrules.Action{
					{
						Kind: pushrules.DontNotifyAction,
					},
				},
			},
			{
				name:         "m.room.message notifies",
				eventContent: `{"type":"m.room.message","room_id":"!room:example.com"}`,
				wantNotify:   true,
				wantAction:   pushrules.NotifyAction,
				wantActions: []*pushrules.Action{
					{Kind: pushrules.NotifyAction},
				},
			},
			{
				name:         "m.room.message highlights",
				eventContent: `{"type":"m.room.message", "content": {"body": "test"},"room_id":"!room:example.com"}`,
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

func TestLocalRoomMembers(t *testing.T) {
	alice := test.NewUser(t)
	_, sk, err := ed25519.GenerateKey(nil)
	assert.NoError(t, err)
	bob := test.NewUser(t, test.WithSigningServer("notlocalhost", "ed25519:abc", sk))
	charlie := test.NewUser(t, test.WithSigningServer("notlocalhost", "ed25519:abc", sk))

	room := test.NewRoom(t, alice)
	room.CreateAndInsert(t, bob, spec.MRoomMember, map[string]string{"membership": spec.Join}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, charlie, spec.MRoomMember, map[string]string{"membership": spec.Join}, test.WithStateKey(charlie.ID))

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		cfg, processCtx, close := testrig.CreateConfig(t, dbType)
		defer close()

		cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
		natsInstance := &jetstream.NATSInstance{}
		caches := caching.NewRistrettoCache(8*1024*1024, time.Hour, caching.DisableMetrics)
		rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
		rsAPI.SetFederationAPI(nil, nil)
		db, err := storage.NewUserDatabase(processCtx.Context(), cm, &cfg.UserAPI.AccountDatabase, cfg.Global.ServerName, bcrypt.MinCost, 1000, 1000, "")
		assert.NoError(t, err)

		err = rsapi.SendEvents(processCtx.Context(), rsAPI, rsapi.KindNew, room.Events(), "", "test", "test", nil, false)
		assert.NoError(t, err)

		consumer := OutputRoomEventConsumer{db: db, rsAPI: rsAPI, serverName: "test", cfg: &cfg.UserAPI}
		members, count, err := consumer.localRoomMembers(processCtx.Context(), room.ID)
		assert.NoError(t, err)
		assert.Equal(t, 3, count)
		expectedLocalMember := &localMembership{UserID: alice.ID, Localpart: alice.Localpart, Domain: "test", MemberContent: gomatrixserverlib.MemberContent{Membership: spec.Join}}
		assert.Equal(t, expectedLocalMember, members[0])
	})

}

func TestMessageStats(t *testing.T) {
	type args struct {
		eventType   string
		eventSender string
		roomID      string
	}
	tests := []struct {
		name           string
		args           args
		ourServer      spec.ServerName
		lastUpdate     time.Time
		initRoomCounts map[spec.ServerName]map[string]bool
		wantStats      userAPITypes.MessageStats
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
				roomID:      "normalRoom",
			},
			wantStats: userAPITypes.MessageStats{Messages: 1, SentMessages: 1},
		},
		{
			name:      "our server - E2EE message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.encrypted",
				eventSender: "@alice:localhost",
				roomID:      "encryptedRoom",
			},
			wantStats: userAPITypes.MessageStats{Messages: 1, SentMessages: 1, MessagesE2EE: 1, SentMessagesE2EE: 1},
		},

		{
			name:      "remote server - message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.message",
				eventSender: "@alice:remote",
				roomID:      "normalRoom",
			},
			wantStats: userAPITypes.MessageStats{Messages: 2, SentMessages: 1, MessagesE2EE: 1, SentMessagesE2EE: 1},
		},
		{
			name:      "remote server - E2EE message",
			ourServer: "localhost",
			args: args{
				eventType:   "m.room.encrypted",
				eventSender: "@alice:remote",
				roomID:      "encryptedRoom",
			},
			wantStats: userAPITypes.MessageStats{Messages: 2, SentMessages: 1, MessagesE2EE: 2, SentMessagesE2EE: 1},
		},
		{
			name:       "day change creates a new room map",
			ourServer:  "localhost",
			lastUpdate: time.Now().Add(-time.Hour * 24),
			initRoomCounts: map[spec.ServerName]map[string]bool{
				"localhost": {"encryptedRoom": true},
			},
			args: args{
				eventType:   "m.room.encrypted",
				eventSender: "@alice:remote",
				roomID:      "someOtherRoom",
			},
			wantStats: userAPITypes.MessageStats{Messages: 2, SentMessages: 1, MessagesE2EE: 3, SentMessagesE2EE: 1},
		},
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.lastUpdate.IsZero() {
					tt.lastUpdate = time.Now()
				}
				if tt.initRoomCounts == nil {
					tt.initRoomCounts = map[spec.ServerName]map[string]bool{}
				}
				s := &OutputRoomEventConsumer{
					db:         db,
					msgCounts:  map[spec.ServerName]userAPITypes.MessageStats{},
					roomCounts: tt.initRoomCounts,
					countsLock: sync.Mutex{},
					lastUpdate: tt.lastUpdate,
					serverName: tt.ourServer,
				}
				s.storeMessageStats(context.Background(), tt.args.eventType, tt.args.eventSender, tt.args.roomID)
				t.Logf("%+v", s.roomCounts)
				gotStats, activeRooms, activeE2EERooms, err := db.DailyRoomsMessages(context.Background(), tt.ourServer)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if !reflect.DeepEqual(gotStats, tt.wantStats) {
					t.Fatalf("expected %+v, got %+v", tt.wantStats, gotStats)
				}
				if tt.args.eventType == "m.room.encrypted" && activeE2EERooms != 1 {
					t.Fatalf("expected room to be activeE2EE")
				}
				if tt.args.eventType == "m.room.message" && activeRooms != 1 {
					t.Fatalf("expected room to be active")
				}
			})
		}
	})
}

func BenchmarkLocalRoomMembers(b *testing.B) {
	t := &testing.T{}

	cfg, processCtx, close := testrig.CreateConfig(t, test.DBTypePostgres)
	defer close()
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	natsInstance := &jetstream.NATSInstance{}
	caches := caching.NewRistrettoCache(8*1024*1024, time.Hour, caching.DisableMetrics)
	rsAPI := roomserver.NewInternalAPI(processCtx, cfg, cm, natsInstance, caches, caching.DisableMetrics)
	rsAPI.SetFederationAPI(nil, nil)
	db, err := storage.NewUserDatabase(processCtx.Context(), cm, &cfg.UserAPI.AccountDatabase, cfg.Global.ServerName, bcrypt.MinCost, 1000, 1000, "")
	assert.NoError(b, err)

	consumer := OutputRoomEventConsumer{db: db, rsAPI: rsAPI, serverName: "test", cfg: &cfg.UserAPI}
	_, sk, err := ed25519.GenerateKey(nil)
	assert.NoError(b, err)

	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	for i := 0; i < 100; i++ {
		user := test.NewUser(t, test.WithSigningServer("notlocalhost", "ed25519:abc", sk))
		room.CreateAndInsert(t, user, spec.MRoomMember, map[string]string{"membership": spec.Join}, test.WithStateKey(user.ID))
	}

	err = rsapi.SendEvents(processCtx.Context(), rsAPI, rsapi.KindNew, room.Events(), "", "test", "test", nil, false)
	assert.NoError(b, err)

	expectedLocalMember := &localMembership{UserID: alice.ID, Localpart: alice.Localpart, Domain: "test", MemberContent: gomatrixserverlib.MemberContent{Membership: spec.Join}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		members, count, err := consumer.localRoomMembers(processCtx.Context(), room.ID)
		assert.NoError(b, err)
		assert.Equal(b, 101, count)
		assert.Equal(b, expectedLocalMember, members[0])
	}
}
