package routing

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi"
	uapi "github.com/matrix-org/dendrite/userapi/api"
)

func TestJoinRoomByIDOrAlias(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t, test.WithAccountType(uapi.AccountTypeGuest))

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, baseClose := testrig.CreateBaseDendrite(t, dbType)
		defer baseClose()

		rsAPI := roomserver.NewInternalAPI(base)
		userAPI := userapi.NewInternalAPI(base, rsAPI, nil)
		asAPI := appservice.NewInternalAPI(base, userAPI, rsAPI)
		rsAPI.SetFederationAPI(nil, nil) // creates the rs.Inputer etc

		// Create the users in the userapi
		for _, u := range []*test.User{alice, bob, charlie} {
			localpart, serverName, _ := gomatrixserverlib.SplitID('@', u.ID)
			userRes := &uapi.PerformAccountCreationResponse{}
			if err := userAPI.PerformAccountCreation(ctx, &uapi.PerformAccountCreationRequest{
				AccountType: u.AccountType,
				Localpart:   localpart,
				ServerName:  serverName,
				Password:    "someRandomPassword",
			}, userRes); err != nil {
				t.Errorf("failed to create account: %s", err)
			}

		}

		aliceDev := &uapi.Device{UserID: alice.ID}
		bobDev := &uapi.Device{UserID: bob.ID}
		charlieDev := &uapi.Device{UserID: charlie.ID, AccountType: uapi.AccountTypeGuest}

		// create a room with disabled guest access and invite Bob
		resp := createRoom(ctx, createRoomRequest{
			Name:          "testing",
			IsDirect:      true,
			Topic:         "testing",
			Visibility:    "public",
			Preset:        presetPublicChat,
			RoomAliasName: "alias",
			Invite:        []string{bob.ID},
			GuestCanJoin:  false,
		}, aliceDev, &base.Cfg.ClientAPI, userAPI, rsAPI, asAPI, time.Now())
		crResp, ok := resp.JSON.(createRoomResponse)
		if !ok {
			t.Fatalf("response is not a createRoomResponse: %+v", resp)
		}

		// create a room with guest access enabled and invite Charlie
		resp = createRoom(ctx, createRoomRequest{
			Name:         "testing",
			IsDirect:     true,
			Topic:        "testing",
			Visibility:   "public",
			Preset:       presetPublicChat,
			Invite:       []string{charlie.ID},
			GuestCanJoin: true,
		}, aliceDev, &base.Cfg.ClientAPI, userAPI, rsAPI, asAPI, time.Now())
		crRespWithGuestAccess, ok := resp.JSON.(createRoomResponse)
		if !ok {
			t.Fatalf("response is not a createRoomResponse: %+v", resp)
		}

		// Dummy request
		body := &bytes.Buffer{}
		req, err := http.NewRequest(http.MethodPost, "/?server_name=test", body)
		if err != nil {
			t.Fatal(err)
		}

		testCases := []struct {
			name        string
			device      *uapi.Device
			roomID      string
			wantHTTP200 bool
		}{
			{
				name:        "User can join successfully by alias",
				device:      bobDev,
				roomID:      crResp.RoomAlias,
				wantHTTP200: true,
			},
			{
				name:        "User can join successfully by roomID",
				device:      bobDev,
				roomID:      crResp.RoomID,
				wantHTTP200: true,
			},
			{
				name:   "join is forbidden if user is guest",
				device: charlieDev,
				roomID: crResp.RoomID,
			},
			{
				name:   "room does not exist",
				device: aliceDev,
				roomID: "!doesnotexist:test",
			},
			{
				name:   "user from different server",
				device: &uapi.Device{UserID: "@wrong:server"},
				roomID: crResp.RoomAlias,
			},
			{
				name:   "user doesn't exist locally",
				device: &uapi.Device{UserID: "@doesnotexist:test"},
				roomID: crResp.RoomAlias,
			},
			{
				name:   "invalid room ID",
				device: aliceDev,
				roomID: "invalidRoomID",
			},
			{
				name:   "roomAlias does not exist",
				device: aliceDev,
				roomID: "#doesnotexist:test",
			},
			{
				name:   "room with guest_access event",
				device: charlieDev,
				roomID: crRespWithGuestAccess.RoomID,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				joinResp := JoinRoomByIDOrAlias(req, tc.device, rsAPI, userAPI, tc.roomID)
				if tc.wantHTTP200 && !joinResp.Is2xx() {
					t.Fatalf("expected join room to succeed, but didn't: %+v", joinResp)
				}
			})
		}
	})
}
