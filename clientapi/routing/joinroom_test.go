package routing

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/appservice"
	"github.com/matrix-org/dendrite/keyserver"
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
		keyAPI := keyserver.NewInternalAPI(base, &base.Cfg.KeyServer, nil)
		userAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, keyAPI, rsAPI, nil)
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

		// create the room
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

		// Dummy request
		body := &bytes.Buffer{}
		req, err := http.NewRequest(http.MethodPost, "/?server_name=test", body)
		if err != nil {
			t.Fatal(err)
		}
		// Bob can join the room as usual
		joinResp := JoinRoomByIDOrAlias(req, bobDev, rsAPI, userAPI, crResp.RoomAlias)
		if !joinResp.Is2xx() {
			t.Fatalf("expected join to succeed, but didn't: %+v", joinResp)
		}

		// Charlie is a guest, and guests are prohibited to join the room
		joinResp = JoinRoomByIDOrAlias(req, charlieDev, rsAPI, userAPI, crResp.RoomID)
		if joinResp.Is2xx() {
			t.Fatalf("expected join to fail, but didn't: %+v", joinResp)
		}

		if joinResp.Code != http.StatusForbidden {
			t.Fatalf("expected response code to be %d, got %d", http.StatusForbidden, joinResp.Code)
		}

	})
}
