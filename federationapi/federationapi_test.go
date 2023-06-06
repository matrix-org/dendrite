package federationapi_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/nats-io/nats.go"

	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/internal"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

type fedRoomserverAPI struct {
	rsapi.FederationRoomserverAPI
	inputRoomEvents   func(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse)
	queryRoomsForUser func(ctx context.Context, req *rsapi.QueryRoomsForUserRequest, res *rsapi.QueryRoomsForUserResponse) error
}

func (f *fedRoomserverAPI) QueryUserIDForSender(ctx context.Context, roomID string, senderID string) (*spec.UserID, error) {
	return spec.NewUserID(senderID, true)
}

// PerformJoin will call this function
func (f *fedRoomserverAPI) InputRoomEvents(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse) {
	if f.inputRoomEvents == nil {
		return
	}
	f.inputRoomEvents(ctx, req, res)
}

// keychange consumer calls this
func (f *fedRoomserverAPI) QueryRoomsForUser(ctx context.Context, req *rsapi.QueryRoomsForUserRequest, res *rsapi.QueryRoomsForUserResponse) error {
	if f.queryRoomsForUser == nil {
		return nil
	}
	return f.queryRoomsForUser(ctx, req, res)
}

// TODO: This struct isn't generic, only works for TestFederationAPIJoinThenKeyUpdate
type fedClient struct {
	fedClientMutex sync.Mutex
	fclient.FederationClient
	allowJoins []*test.Room
	keys       map[spec.ServerName]struct {
		key   ed25519.PrivateKey
		keyID gomatrixserverlib.KeyID
	}
	t       *testing.T
	sentTxn bool
}

func (f *fedClient) GetServerKeys(ctx context.Context, matrixServer spec.ServerName) (gomatrixserverlib.ServerKeys, error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	fmt.Println("GetServerKeys:", matrixServer)
	var keys gomatrixserverlib.ServerKeys
	var keyID gomatrixserverlib.KeyID
	var pkey ed25519.PrivateKey
	for srv, data := range f.keys {
		if srv == matrixServer {
			pkey = data.key
			keyID = data.keyID
			break
		}
	}
	if pkey == nil {
		return keys, nil
	}

	keys.ServerName = matrixServer
	keys.ValidUntilTS = spec.AsTimestamp(time.Now().Add(10 * time.Hour))
	publicKey := pkey.Public().(ed25519.PublicKey)
	keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
		keyID: {
			Key: spec.Base64Bytes(publicKey),
		},
	}
	toSign, err := json.Marshal(keys.ServerKeyFields)
	if err != nil {
		return keys, err
	}

	keys.Raw, err = gomatrixserverlib.SignJSON(
		string(matrixServer), keyID, pkey, toSign,
	)
	if err != nil {
		return keys, err
	}

	return keys, nil
}

func (f *fedClient) MakeJoin(ctx context.Context, origin, s spec.ServerName, roomID, userID string) (res fclient.RespMakeJoin, err error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	for _, r := range f.allowJoins {
		if r.ID == roomID {
			res.RoomVersion = r.Version
			res.JoinEvent = gomatrixserverlib.ProtoEvent{
				Sender:     userID,
				RoomID:     roomID,
				Type:       "m.room.member",
				StateKey:   &userID,
				Content:    spec.RawJSON([]byte(`{"membership":"join"}`)),
				PrevEvents: r.ForwardExtremities(),
			}
			var needed gomatrixserverlib.StateNeeded
			needed, err = gomatrixserverlib.StateNeededForProtoEvent(&res.JoinEvent)
			if err != nil {
				f.t.Errorf("StateNeededForEventBuilder: %v", err)
				return
			}
			res.JoinEvent.AuthEvents = r.MustGetAuthEventRefsForEvent(f.t, needed)
			return
		}
	}
	return
}
func (f *fedClient) SendJoin(ctx context.Context, origin, s spec.ServerName, event gomatrixserverlib.PDU) (res fclient.RespSendJoin, err error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	for _, r := range f.allowJoins {
		if r.ID == event.RoomID() {
			r.InsertEvent(f.t, &types.HeaderedEvent{PDU: event})
			f.t.Logf("Join event: %v", event.EventID())
			res.StateEvents = types.NewEventJSONsFromHeaderedEvents(r.CurrentState())
			res.AuthEvents = types.NewEventJSONsFromHeaderedEvents(r.Events())
		}
	}
	return
}

func (f *fedClient) SendTransaction(ctx context.Context, t gomatrixserverlib.Transaction) (res fclient.RespSend, err error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	for _, edu := range t.EDUs {
		if edu.Type == spec.MDeviceListUpdate {
			f.sentTxn = true
		}
	}
	f.t.Logf("got /send")
	return
}

// Regression test to make sure that /send_join is updating the destination hosts synchronously and
// isn't relying on the roomserver.
func TestFederationAPIJoinThenKeyUpdate(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		testFederationAPIJoinThenKeyUpdate(t, dbType)
	})
}

func testFederationAPIJoinThenKeyUpdate(t *testing.T, dbType test.DBType) {
	cfg, processCtx, close := testrig.CreateConfig(t, dbType)
	cm := sqlutil.NewConnectionManager(processCtx, cfg.Global.DatabaseOptions)
	caches := caching.NewRistrettoCache(128*1024*1024, time.Hour, caching.DisableMetrics)
	natsInstance := jetstream.NATSInstance{}
	cfg.FederationAPI.PreferDirectFetch = true
	cfg.FederationAPI.KeyPerspectives = nil
	defer close()
	jsctx, _ := natsInstance.Prepare(processCtx, &cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &cfg.Global.JetStream)

	serverA := spec.ServerName("server.a")
	serverAKeyID := gomatrixserverlib.KeyID("ed25519:servera")
	serverAPrivKey := test.PrivateKeyA
	creator := test.NewUser(t, test.WithSigningServer(serverA, serverAKeyID, serverAPrivKey))

	myServer := cfg.Global.ServerName
	myServerKeyID := cfg.Global.KeyID
	myServerPrivKey := cfg.Global.PrivateKey
	joiningUser := test.NewUser(t, test.WithSigningServer(myServer, myServerKeyID, myServerPrivKey))
	fmt.Printf("creator: %v joining user: %v\n", creator.ID, joiningUser.ID)
	room := test.NewRoom(t, creator)

	rsapi := &fedRoomserverAPI{
		inputRoomEvents: func(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse) {
			if req.Asynchronous {
				t.Errorf("InputRoomEvents from PerformJoin MUST be synchronous")
			}
		},
		queryRoomsForUser: func(ctx context.Context, req *rsapi.QueryRoomsForUserRequest, res *rsapi.QueryRoomsForUserResponse) error {
			if req.UserID == joiningUser.ID && req.WantMembership == "join" {
				res.RoomIDs = []string{room.ID}
				return nil
			}
			return fmt.Errorf("unexpected queryRoomsForUser: %+v", *req)
		},
	}
	fc := &fedClient{
		allowJoins: []*test.Room{room},
		t:          t,
		keys: map[spec.ServerName]struct {
			key   ed25519.PrivateKey
			keyID gomatrixserverlib.KeyID
		}{
			serverA: {
				key:   serverAPrivKey,
				keyID: serverAKeyID,
			},
			myServer: {
				key:   myServerPrivKey,
				keyID: myServerKeyID,
			},
		},
	}
	fsapi := federationapi.NewInternalAPI(processCtx, cfg, cm, &natsInstance, fc, rsapi, caches, nil, false)

	var resp api.PerformJoinResponse
	fsapi.PerformJoin(context.Background(), &api.PerformJoinRequest{
		RoomID:      room.ID,
		UserID:      joiningUser.ID,
		ServerNames: []spec.ServerName{serverA},
	}, &resp)
	if resp.JoinedVia != serverA {
		t.Errorf("PerformJoin: joined via %v want %v", resp.JoinedVia, serverA)
	}
	if resp.LastError != nil {
		t.Fatalf("PerformJoin: returned error: %+v", *resp.LastError)
	}

	// Inject a keyserver key change event and ensure we try to send it out. If we don't, then the
	// federationapi is incorrectly waiting for an output room event to arrive to update the joined
	// hosts table.
	key := userapi.DeviceMessage{
		Type: userapi.TypeDeviceKeyUpdate,
		DeviceKeys: &userapi.DeviceKeys{
			UserID:      joiningUser.ID,
			DeviceID:    "MY_DEVICE",
			DisplayName: "BLARGLE",
			KeyJSON:     []byte(`{}`),
		},
	}
	b, err := json.Marshal(key)
	if err != nil {
		t.Fatalf("Failed to marshal device message: %s", err)
	}

	msg := &nats.Msg{
		Subject: cfg.Global.JetStream.Prefixed(jetstream.OutputKeyChangeEvent),
		Header:  nats.Header{},
		Data:    b,
	}
	msg.Header.Set(jetstream.UserID, key.UserID)

	testrig.MustPublishMsgs(t, jsctx, msg)
	time.Sleep(500 * time.Millisecond)
	fc.fedClientMutex.Lock()
	defer fc.fedClientMutex.Unlock()
	if !fc.sentTxn {
		t.Fatalf("did not send device list update")
	}
}

// Tests that event IDs with '/' in them (escaped as %2F) are correctly passed to the right handler and don't 404.
// Relevant for v3 rooms and a cause of flakey sytests as the IDs are randomly generated.
func TestRoomsV3URLEscapeDoNot404(t *testing.T) {
	testCases := []struct {
		roomVer   gomatrixserverlib.RoomVersion
		eventJSON string
	}{
		{
			eventJSON: `{"auth_events":[["$Nzfbrhc3oaYVKzGM:localhost",{"sha256":"BCBHOgB4qxLPQkBd6th8ydFSyqjth/LF99VNjYffOQ0"}],["$EZzkD2BH1Gtm5v1D:localhost",{"sha256":"3dLUnDBs8/iC5DMw/ydKtmAqVZtzqqtHpsjsQPk7GJA"}]],"content":{"body":"Test Message"},"depth":11,"event_id":"$mGiPO3oGjQfCkIUw:localhost","hashes":{"sha256":"h+t+4DwIBC9UNyJ3jzyAQAAl4H3yQHVuHrm2S1JZizU"},"origin":"localhost","origin_server_ts":0,"prev_events":[["$tFr64vpiSHdLU0Qr:localhost",{"sha256":"+R07ZrIs4c4tjPFE+tmcYIGUfeLGFI/4e0OITb9uEcM"}]],"room_id":"!roomid:localhost","sender":"@userid:localhost","signatures":{"localhost":{"ed25519:auto":"LYFr/rW9m5/7UKBQMF5qWnG82He4VGsRESUgDmvkn5DrJRyS4TLL/7zl0Lymn3pa3q2yaTO74LQX/CRotqG1BA"}},"type":"m.room.message"}`,
			roomVer:   gomatrixserverlib.RoomVersionV1,
		},
		// single / (handlers which do not UseEncodedPath will fail this test)
		// EventID: $0SFh2WJbjBs3OT+E0yl95giDKo/3Zp52HsHUUk4uPyg
		{
			eventJSON: `{"auth_events":["$x4MKEPRSF6OGlo0qpnsP3BfSmYX5HhVlykOsQH3ECyg","$BcEcbZnlFLB5rxSNSZNBn6fO3jU/TKAJ79wfKyCQLiU"],"content":{"body":"Test Message"},"depth":8,"hashes":{"sha256":"dfK0MBn1RZZqCVJqWsn/MGY7QJHjQcwqF0unOonLCTU"},"origin":"localhost","origin_server_ts":0,"prev_events":["$1SwcZ1XY/Y8yKLjP4DzAOHN5WFBcDAZxb5vFDnW2ubA"],"room_id":"!roomid:localhost","sender":"@userid:localhost","signatures":{"localhost":{"ed25519:auto":"INOjuWMg+GmFkUpmzhMB0bqLNs73mSvwldY1ftYIQ/B3lD9soD2OMG3AF+wgZW/I8xqzY4DOHfbnbUeYPf67BA"}},"type":"m.room.message"}`,
			roomVer:   gomatrixserverlib.RoomVersionV3,
		},
		// multiple /
		// EventID: $OzENBCuVv/fnRAYCeQudIon/84/V5pxtEjQMTgi3emk
		{
			eventJSON: `{"auth_events":["$x4MKEPRSF6OGlo0qpnsP3BfSmYX5HhVlykOsQH3ECyg","$BcEcbZnlFLB5rxSNSZNBn6fO3jU/TKAJ79wfKyCQLiU"],"content":{"body":"Test Message"},"depth":2,"hashes":{"sha256":"U5+WsiJAhiEM88J8HTjuUjPImVGVzDFD3v/WS+jb2f0"},"origin":"localhost","origin_server_ts":0,"prev_events":["$BcEcbZnlFLB5rxSNSZNBn6fO3jU/TKAJ79wfKyCQLiU"],"room_id":"!roomid:localhost","sender":"@userid:localhost","signatures":{"localhost":{"ed25519:auto":"tKS469e9+wdWPEKB/LbBJWQ8vfOOdKgTWER5IwbSAH1CxmLvkCziUsgVu85zfzDSLoUi5mU5FHLiMTC6P/qICw"}},"type":"m.room.message"}`,
			roomVer:   gomatrixserverlib.RoomVersionV3,
		},
		// two slashes (handlers which clean paths before UseEncodedPath will fail this test)
		// EventID: $EmwNBlHoSOVmCZ1cM//yv/OvxB6r4OFEIGSJea7+Amk
		{
			eventJSON: `{"auth_events":["$x4MKEPRSF6OGlo0qpnsP3BfSmYX5HhVlykOsQH3ECyg","$BcEcbZnlFLB5rxSNSZNBn6fO3jU/TKAJ79wfKyCQLiU"],"content":{"body":"Test Message"},"depth":3917,"hashes":{"sha256":"cNAWtlHIegrji0mMA6x1rhpYCccY8W1NsWZqSpJFhjs"},"origin":"localhost","origin_server_ts":0,"prev_events":["$4GDB0bVjkWwS3G4noUZCq5oLWzpBYpwzdMcf7gj24CI"],"room_id":"!roomid:localhost","sender":"@userid:localhost","signatures":{"localhost":{"ed25519:auto":"NKym6Kcy3u9mGUr21Hjfe3h7DfDilDhN5PqztT0QZ4NTZ+8Y7owseLolQVXp+TvNjecvzdDywsXXVvGiuQiWAQ"}},"type":"m.room.message"}`,
			roomVer:   gomatrixserverlib.RoomVersionV3,
		},
	}

	cfg, processCtx, close := testrig.CreateConfig(t, test.DBTypeSQLite)
	defer close()
	routers := httputil.NewRouters()

	_, privKey, _ := ed25519.GenerateKey(nil)
	cfg.Global.KeyID = gomatrixserverlib.KeyID("ed25519:auto")
	cfg.Global.ServerName = spec.ServerName("localhost")
	cfg.Global.PrivateKey = privKey
	cfg.Global.JetStream.InMemory = true
	keyRing := &test.NopJSONVerifier{}
	natsInstance := jetstream.NATSInstance{}
	// TODO: This is pretty fragile, as if anything calls anything on these nils this test will break.
	// Unfortunately, it makes little sense to instantiate these dependencies when we just want to test routing.
	federationapi.AddPublicRoutes(processCtx, routers, cfg, &natsInstance, nil, nil, keyRing, nil, &internal.FederationInternalAPI{}, caching.DisableMetrics)
	baseURL, cancel := test.ListenAndServe(t, routers.Federation, true)
	defer cancel()
	serverName := spec.ServerName(strings.TrimPrefix(baseURL, "https://"))

	fedCli := fclient.NewFederationClient(
		cfg.Global.SigningIdentities(),
		fclient.WithSkipVerify(true),
	)

	for _, tc := range testCases {
		ev, err := gomatrixserverlib.MustGetRoomVersion(tc.roomVer).NewEventFromTrustedJSON([]byte(tc.eventJSON), false)
		if err != nil {
			t.Errorf("failed to parse event: %s", err)
		}
		invReq, err := fclient.NewInviteV2Request(ev, nil)
		if err != nil {
			t.Errorf("failed to create invite v2 request: %s", err)
			continue
		}
		_, err = fedCli.SendInviteV2(context.Background(), cfg.Global.ServerName, serverName, invReq)
		if err == nil {
			t.Errorf("expected an error, got none")
			continue
		}
		gerr, ok := err.(gomatrix.HTTPError)
		if !ok {
			t.Errorf("failed to cast response error as gomatrix.HTTPError: %s", err)
			continue
		}
		t.Logf("Error: %+v", gerr)
		if gerr.Code == 404 {
			t.Errorf("invite event resulted in a 404")
		}
	}
}
