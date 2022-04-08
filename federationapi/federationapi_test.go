package federationapi_test

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"

	"github.com/matrix-org/dendrite/federationapi"
	"github.com/matrix-org/dendrite/internal/test"
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrix"
	"github.com/matrix-org/gomatrixserverlib"
)

// Tests that event IDs with '/' in them (escaped as %2F) are correctly passed to the right handler and don't 404.
// Relevant for v3 rooms and a cause of flakey sytests as the IDs are randomly generated.
func TestRoomsV3URLEscapeDoNot404(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	cfg := &config.Dendrite{}
	cfg.Defaults(true)
	cfg.Global.KeyID = gomatrixserverlib.KeyID("ed25519:auto")
	cfg.Global.ServerName = gomatrixserverlib.ServerName("localhost")
	cfg.Global.PrivateKey = privKey
	cfg.Global.JetStream.InMemory = true
	cfg.FederationAPI.Database.ConnectionString = config.DataSource("file::memory:")
	base := base.NewBaseDendrite(cfg, "Monolith")
	keyRing := &test.NopJSONVerifier{}
	fsAPI := base.FederationAPIHTTPClient()
	// TODO: This is pretty fragile, as if anything calls anything on these nils this test will break.
	// Unfortunately, it makes little sense to instantiate these dependencies when we just want to test routing.
	federationapi.AddPublicRoutes(base.ProcessContext, base.PublicFederationAPIMux, base.PublicKeyAPIMux, base.PublicWellKnownAPIMux, &cfg.FederationAPI, nil, nil, keyRing, nil, fsAPI, nil, &cfg.MSCs, nil)
	baseURL, cancel := test.ListenAndServe(t, base.PublicFederationAPIMux, true)
	defer cancel()
	serverName := gomatrixserverlib.ServerName(strings.TrimPrefix(baseURL, "https://"))

	fedCli := gomatrixserverlib.NewFederationClient(
		serverName, cfg.Global.KeyID, cfg.Global.PrivateKey,
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
		_, err = fedCli.SendInviteV2(context.Background(), serverName, invReq)
		if err == nil {
			t.Errorf("expected an error, got none")
			continue
		}
		gerr, ok := err.(gomatrix.HTTPError)
		if !ok {
			t.Errorf("failed to cast response error as gomatrix.HTTPError")
			continue
		}
		t.Logf("Error: %+v", gerr)
		if gerr.Code == 404 {
			t.Errorf("invite event resulted in a 404")
		}
	}
}
