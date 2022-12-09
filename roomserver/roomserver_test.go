package roomserver_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/userapi"

	userAPI "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/gomatrixserverlib"

	"github.com/matrix-org/dendrite/roomserver"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

func TestUsers(t *testing.T) {

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		base, close := testrig.CreateBaseDendrite(t, dbType)
		defer close()
		rsAPI := roomserver.NewInternalAPI(base)
		// SetFederationAPI starts the room event input consumer
		rsAPI.SetFederationAPI(nil, nil)

		t.Run("shared users", func(t *testing.T) {
			testSharedUsers(t, rsAPI)
		})

		t.Run("kick users", func(t *testing.T) {
			usrAPI := userapi.NewInternalAPI(base, &base.Cfg.UserAPI, nil, nil, rsAPI, nil)
			rsAPI.SetUserAPI(usrAPI)
			testKickUsers(t, rsAPI, usrAPI)
		})
	})
}

func testSharedUsers(t *testing.T, rsAPI api.RoomserverInternalAPI) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetTrustedPrivateChat))

	// Invite and join Bob
	room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "invite",
	}, test.WithStateKey(bob.ID))
	room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()

	// Create the room
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// Query the shared users for Alice, there should only be Bob.
	// This is used by the SyncAPI keychange consumer.
	res := &api.QuerySharedUsersResponse{}
	if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID}, res); err != nil {
		t.Errorf("unable to query known users: %v", err)
	}
	if _, ok := res.UserIDsToCount[bob.ID]; !ok {
		t.Errorf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
	}
	// Also verify that we get the expected result when specifying OtherUserIDs.
	// This is used by the SyncAPI when getting device list changes.
	if err := rsAPI.QuerySharedUsers(ctx, &api.QuerySharedUsersRequest{UserID: alice.ID, OtherUserIDs: []string{bob.ID}}, res); err != nil {
		t.Errorf("unable to query known users: %v", err)
	}
	if _, ok := res.UserIDsToCount[bob.ID]; !ok {
		t.Errorf("expected to find %s in shared users, but didn't: %+v", bob.ID, res.UserIDsToCount)
	}
}

func testKickUsers(t *testing.T, rsAPI api.RoomserverInternalAPI, usrAPI userAPI.UserInternalAPI) {
	// Create users and room; Bob is going to be the guest and kicked on revocation of guest access
	alice := test.NewUser(t, test.WithAccountType(userAPI.AccountTypeUser))
	bob := test.NewUser(t, test.WithAccountType(userAPI.AccountTypeGuest))

	room := test.NewRoom(t, alice, test.RoomPreset(test.PresetPublicChat), test.GuestsCanJoin(true))

	// Join with the guest user
	room.CreateAndInsert(t, bob, gomatrixserverlib.MRoomMember, map[string]interface{}{
		"membership": "join",
	}, test.WithStateKey(bob.ID))

	ctx := context.Background()

	// Create the users in the userapi, so the RSAPI can query the account type later
	for _, u := range []*test.User{alice, bob} {
		localpart, serverName, _ := gomatrixserverlib.SplitID('@', u.ID)
		userRes := &userAPI.PerformAccountCreationResponse{}
		if err := usrAPI.PerformAccountCreation(ctx, &userAPI.PerformAccountCreationRequest{
			AccountType: u.AccountType,
			Localpart:   localpart,
			ServerName:  serverName,
			Password:    "someRandomPassword",
		}, userRes); err != nil {
			t.Errorf("failed to create account: %s", err)
		}
	}

	// Create the room in the database
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, room.Events(), "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// Get the membership events BEFORE revoking guest access
	membershipRes := &api.QueryMembershipsForRoomResponse{}
	if err := rsAPI.QueryMembershipsForRoom(ctx, &api.QueryMembershipsForRoomRequest{LocalOnly: true, JoinedOnly: true, RoomID: room.ID}, membershipRes); err != nil {
		t.Errorf("failed to query membership for room: %s", err)
	}

	// revoke guest access
	revokeEvent := room.CreateAndInsert(t, alice, gomatrixserverlib.MRoomGuestAccess, map[string]string{"guest_access": "forbidden"}, test.WithStateKey(""))
	if err := api.SendEvents(ctx, rsAPI, api.KindNew, []*gomatrixserverlib.HeaderedEvent{revokeEvent}, "test", "test", "test", nil, false); err != nil {
		t.Errorf("failed to send events: %v", err)
	}

	// TODO: Even though we are sending the events sync, the "kickUsers" function is sending the events async, so we need
	//		 to loop and wait for the events to be processed by the roomserver.
	for i := 0; i <= 20; i++ {
		// Get the membership events AFTER revoking guest access
		membershipRes2 := &api.QueryMembershipsForRoomResponse{}
		if err := rsAPI.QueryMembershipsForRoom(ctx, &api.QueryMembershipsForRoomRequest{LocalOnly: true, JoinedOnly: true, RoomID: room.ID}, membershipRes2); err != nil {
			t.Errorf("failed to query membership for room: %s", err)
		}

		// The membership events should NOT match, as Bob (guest user) should now be kicked from the room
		if !reflect.DeepEqual(membershipRes, membershipRes2) {
			return
		}
		time.Sleep(time.Millisecond * 10)
	}

	t.Errorf("memberships didn't change in time")
}
