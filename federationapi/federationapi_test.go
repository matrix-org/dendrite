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

	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/nats-io/nats.go"

	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/federationapi/api"
	"github.com/matrix-org/dendrite/federationapi/internal"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	rsapi "github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/jetstream"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

type fedRoomserverAPI struct {
	rsapi.FederationRoomserverAPI
	inputRoomEvents   func(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse)
	queryRoomsForUser func(ctx context.Context, req *rsapi.QueryRoomsForUserRequest, res *rsapi.QueryRoomsForUserResponse) error
}

// PerformJoin will call this function
func (f *fedRoomserverAPI) InputRoomEvents(ctx context.Context, req *rsapi.InputRoomEventsRequest, res *rsapi.InputRoomEventsResponse) error {
	if f.inputRoomEvents == nil {
		return nil
	}
	f.inputRoomEvents(ctx, req, res)
	return nil
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
	api.FederationClient
	allowJoins []*test.Room
	keys       map[gomatrixserverlib.ServerName]struct {
		key   ed25519.PrivateKey
		keyID gomatrixserverlib.KeyID
	}
	t       *testing.T
	sentTxn bool
}

func (f *fedClient) GetServerKeys(ctx context.Context, matrixServer gomatrixserverlib.ServerName) (gomatrixserverlib.ServerKeys, error) {
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
	keys.ValidUntilTS = gomatrixserverlib.AsTimestamp(time.Now().Add(10 * time.Hour))
	publicKey := pkey.Public().(ed25519.PublicKey)
	keys.VerifyKeys = map[gomatrixserverlib.KeyID]gomatrixserverlib.VerifyKey{
		keyID: {
			Key: gomatrixserverlib.Base64Bytes(publicKey),
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

func (f *fedClient) MakeJoin(ctx context.Context, origin, s gomatrixserverlib.ServerName, roomID, userID string, roomVersions []gomatrixserverlib.RoomVersion) (res gomatrixserverlib.RespMakeJoin, err error) {
	for _, r := range f.allowJoins {
		if r.ID == roomID {
			res.RoomVersion = r.Version
			res.JoinEvent = gomatrixserverlib.EventBuilder{
				Sender:     userID,
				RoomID:     roomID,
				Type:       "m.room.member",
				StateKey:   &userID,
				Content:    gomatrixserverlib.RawJSON([]byte(`{"membership":"join"}`)),
				PrevEvents: r.ForwardExtremities(),
			}
			var needed gomatrixserverlib.StateNeeded
			needed, err = gomatrixserverlib.StateNeededForEventBuilder(&res.JoinEvent)
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
func (f *fedClient) SendJoin(ctx context.Context, origin, s gomatrixserverlib.ServerName, event *gomatrixserverlib.Event) (res gomatrixserverlib.RespSendJoin, err error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	for _, r := range f.allowJoins {
		if r.ID == event.RoomID() {
			r.InsertEvent(f.t, event.Headered(r.Version))
			f.t.Logf("Join event: %v", event.EventID())
			res.StateEvents = gomatrixserverlib.NewEventJSONsFromHeaderedEvents(r.CurrentState())
			res.AuthEvents = gomatrixserverlib.NewEventJSONsFromHeaderedEvents(r.Events())
		}
	}
	return
}

func (f *fedClient) SendTransaction(ctx context.Context, t gomatrixserverlib.Transaction) (res gomatrixserverlib.RespSend, err error) {
	f.fedClientMutex.Lock()
	defer f.fedClientMutex.Unlock()
	for _, edu := range t.EDUs {
		if edu.Type == gomatrixserverlib.MDeviceListUpdate {
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
	base, close := testrig.CreateBaseDendrite(t, dbType)
	base.Cfg.FederationAPI.PreferDirectFetch = true
	base.Cfg.FederationAPI.KeyPerspectives = nil
	defer close()
	jsctx, _ := base.NATS.Prepare(base.ProcessContext, &base.Cfg.Global.JetStream)
	defer jetstream.DeleteAllStreams(jsctx, &base.Cfg.Global.JetStream)

	serverA := gomatrixserverlib.ServerName("server.a")
	serverAKeyID := gomatrixserverlib.KeyID("ed25519:servera")
	serverAPrivKey := test.PrivateKeyA
	creator := test.NewUser(t, test.WithSigningServer(serverA, serverAKeyID, serverAPrivKey))

	myServer := base.Cfg.Global.ServerName
	myServerKeyID := base.Cfg.Global.KeyID
	myServerPrivKey := base.Cfg.Global.PrivateKey
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
		keys: map[gomatrixserverlib.ServerName]struct {
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
	fsapi := federationapi.NewInternalAPI(base, fc, rsapi, base.Caches, nil, false)

	var resp api.PerformJoinResponse
	fsapi.PerformJoin(context.Background(), &api.PerformJoinRequest{
		RoomID:      room.ID,
		UserID:      joiningUser.ID,
		ServerNames: []gomatrixserverlib.ServerName{serverA},
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
	key := keyapi.DeviceMessage{
		Type: keyapi.TypeDeviceKeyUpdate,
		DeviceKeys: &keyapi.DeviceKeys{
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
		Subject: base.Cfg.Global.JetStream.Prefixed(jetstream.OutputKeyChangeEvent),
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
	_, privKey, _ := ed25519.GenerateKey(nil)
	cfg := &config.Dendrite{}
	cfg.Defaults(config.DefaultOpts{
		Generate:   true,
		Monolithic: true,
	})
	cfg.Global.KeyID = gomatrixserverlib.KeyID("ed25519:auto")
	cfg.Global.ServerName = gomatrixserverlib.ServerName("localhost")
	cfg.Global.PrivateKey = privKey
	cfg.Global.JetStream.InMemory = true
	base := base.NewBaseDendrite(cfg, "Monolith")
	keyRing := &test.NopJSONVerifier{}
	// TODO: This is pretty fragile, as if anything calls anything on these nils this test will break.
	// Unfortunately, it makes little sense to instantiate these dependencies when we just want to test routing.
	federationapi.AddPublicRoutes(base, nil, nil, keyRing, nil, &internal.FederationInternalAPI{}, nil, nil)
	baseURL, cancel := test.ListenAndServe(t, base.PublicFederationAPIMux, true)
	defer cancel()
	serverName := gomatrixserverlib.ServerName(strings.TrimPrefix(baseURL, "https://"))

	fedCli := gomatrixserverlib.NewFederationClient(
		cfg.Global.SigningIdentities(),
		gomatrixserverlib.WithSkipVerify(true),
	)

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

	for _, tc := range testCases {
		ev, err := gomatrixserverlib.NewEventFromTrustedJSON([]byte(tc.eventJSON), false, tc.roomVer)
		if err != nil {
			t.Errorf("failed to parse event: %s", err)
		}
		he := ev.Headered(tc.roomVer)
		invReq, err := gomatrixserverlib.NewInviteV2Request(he, nil)
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
