package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/dendrite/userapi/storage/tables"
)

const loginTokenLifetime = time.Minute

var (
	openIDLifetimeMS = time.Minute.Milliseconds()
	ctx              = context.Background()
)

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	base, baseclose := testrig.CreateBaseDendrite(t, dbType)
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := storage.NewUserAPIDatabase(base, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, "localhost", bcrypt.MinCost, openIDLifetimeMS, loginTokenLifetime, "_server")
	if err != nil {
		t.Fatalf("NewUserAPIDatabase returned %s", err)
	}
	return db, func() {
		close()
		baseclose()
	}
}

// Tests storing and getting account data
func Test_AccountData(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser(t)
		localpart, domain, err := gomatrixserverlib.SplitID('@', alice.ID)
		assert.NoError(t, err)

		room := test.NewRoom(t, alice)
		events := room.Events()

		contentRoom := json.RawMessage(fmt.Sprintf(`{"event_id":"%s"}`, events[len(events)-1].EventID()))
		err = db.SaveAccountData(ctx, localpart, domain, room.ID, "m.fully_read", contentRoom)
		assert.NoError(t, err, "unable to save account data")

		contentGlobal := json.RawMessage(fmt.Sprintf(`{"recent_rooms":["%s"]}`, room.ID))
		err = db.SaveAccountData(ctx, localpart, domain, "", "im.vector.setting.breadcrumbs", contentGlobal)
		assert.NoError(t, err, "unable to save account data")

		accountData, err := db.GetAccountDataByType(ctx, localpart, domain, room.ID, "m.fully_read")
		assert.NoError(t, err, "unable to get account data by type")
		assert.Equal(t, contentRoom, accountData)

		globalData, roomData, err := db.GetAccountData(ctx, localpart, domain)
		assert.NoError(t, err)
		assert.Equal(t, contentRoom, roomData[room.ID]["m.fully_read"])
		assert.Equal(t, contentGlobal, globalData["im.vector.setting.breadcrumbs"])
	})
}

// Tests the creation of accounts
func Test_Accounts(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser(t)
		aliceLocalpart, aliceDomain, err := gomatrixserverlib.SplitID('@', alice.ID)
		assert.NoError(t, err)

		accAlice, err := db.CreateAccount(ctx, aliceLocalpart, aliceDomain, "testing", "", api.AccountTypeAdmin)
		assert.NoError(t, err, "failed to create account")
		// verify the newly create account is the same as returned by CreateAccount
		var accGet *api.Account
		accGet, err = db.GetAccountByPassword(ctx, aliceLocalpart, aliceDomain, "testing")
		assert.NoError(t, err, "failed to get account by password")
		assert.Equal(t, accAlice, accGet)
		accGet, err = db.GetAccountByLocalpart(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "failed to get account by localpart")
		assert.Equal(t, accAlice, accGet)

		// check account availability
		available, err := db.CheckAccountAvailability(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "failed to checkout account availability")
		assert.Equal(t, false, available)

		available, err = db.CheckAccountAvailability(ctx, "unusedname", aliceDomain)
		assert.NoError(t, err, "failed to checkout account availability")
		assert.Equal(t, true, available)

		// get guest account numeric aliceLocalpart
		first, err := db.GetNewNumericLocalpart(ctx, aliceDomain)
		assert.NoError(t, err, "failed to get new numeric localpart")
		// Create a new account to verify the numeric localpart is updated
		_, err = db.CreateAccount(ctx, "", aliceDomain, "testing", "", api.AccountTypeGuest)
		assert.NoError(t, err, "failed to create account")
		second, err := db.GetNewNumericLocalpart(ctx, aliceDomain)
		assert.NoError(t, err)
		assert.Greater(t, second, first)

		// update password for alice
		err = db.SetPassword(ctx, aliceLocalpart, aliceDomain, "newPassword")
		assert.NoError(t, err, "failed to update password")
		accGet, err = db.GetAccountByPassword(ctx, aliceLocalpart, aliceDomain, "newPassword")
		assert.NoError(t, err, "failed to get account by new password")
		assert.Equal(t, accAlice, accGet)

		// deactivate account
		err = db.DeactivateAccount(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "failed to deactivate account")
		// This should fail now, as the account is deactivated
		_, err = db.GetAccountByPassword(ctx, aliceLocalpart, aliceDomain, "newPassword")
		assert.Error(t, err, "expected an error, got none")

		_, err = db.GetAccountByLocalpart(ctx, "unusename", aliceDomain)
		assert.Error(t, err, "expected an error for non existent localpart")

		// create an empty localpart; this should never happen, but is required to test getting a numeric localpart
		// if there's already a user without a localpart in the database
		_, err = db.CreateAccount(ctx, "", aliceDomain, "", "", api.AccountTypeUser)
		assert.NoError(t, err)

		// test getting a numeric localpart, with an existing user without a localpart
		_, err = db.CreateAccount(ctx, "", aliceDomain, "", "", api.AccountTypeGuest)
		assert.NoError(t, err)

		// Create a user with a high numeric localpart, out of range for the Postgres integer (2147483647) type
		_, err = db.CreateAccount(ctx, "2147483650", aliceDomain, "", "", api.AccountTypeUser)
		assert.NoError(t, err)

		// Now try to create a new guest user
		_, err = db.CreateAccount(ctx, "", aliceDomain, "", "", api.AccountTypeGuest)
		assert.NoError(t, err)
	})
}

func Test_Devices(t *testing.T) {
	alice := test.NewUser(t)
	localpart, domain, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)
	deviceID := util.RandomString(8)
	accessToken := util.RandomString(16)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		deviceWithID, err := db.CreateDevice(ctx, localpart, domain, &deviceID, accessToken, nil, "", "")
		assert.NoError(t, err, "unable to create deviceWithoutID")

		gotDevice, err := db.GetDeviceByID(ctx, localpart, domain, deviceID)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, deviceWithID.ID, gotDevice.ID) // GetDeviceByID doesn't populate all fields

		gotDeviceAccessToken, err := db.GetDeviceByAccessToken(ctx, accessToken)
		assert.NoError(t, err, "unable to get device by access token")
		assert.Equal(t, deviceWithID.ID, gotDeviceAccessToken.ID) // GetDeviceByAccessToken doesn't populate all fields

		// create a device without existing device ID
		accessToken = util.RandomString(16)
		deviceWithoutID, err := db.CreateDevice(ctx, localpart, domain, nil, accessToken, nil, "", "")
		assert.NoError(t, err, "unable to create deviceWithoutID")
		gotDeviceWithoutID, err := db.GetDeviceByID(ctx, localpart, domain, deviceWithoutID.ID)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, deviceWithoutID.ID, gotDeviceWithoutID.ID) // GetDeviceByID doesn't populate all fields

		// Get devices
		devices, err := db.GetDevicesByLocalpart(ctx, localpart, domain)
		assert.NoError(t, err, "unable to get devices by localpart")
		assert.Equal(t, 2, len(devices))
		deviceIDs := make([]string, 0, len(devices))
		for _, dev := range devices {
			deviceIDs = append(deviceIDs, dev.ID)
		}

		devices2, err := db.GetDevicesByID(ctx, deviceIDs)
		assert.NoError(t, err, "unable to get devices by id")
		assert.ElementsMatch(t, devices, devices2)

		// Update device
		newName := "new display name"
		err = db.UpdateDevice(ctx, localpart, domain, deviceWithID.ID, &newName)
		assert.NoError(t, err, "unable to update device displayname")
		updatedAfterTimestamp := time.Now().Unix()
		err = db.UpdateDeviceLastSeen(ctx, localpart, domain, deviceWithID.ID, "127.0.0.1", "Element Web")
		assert.NoError(t, err, "unable to update device last seen")

		deviceWithID.DisplayName = newName
		deviceWithID.LastSeenIP = "127.0.0.1"
		gotDevice, err = db.GetDeviceByID(ctx, localpart, domain, deviceWithID.ID)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, 2, len(devices))
		assert.Equal(t, deviceWithID.DisplayName, gotDevice.DisplayName)
		assert.Equal(t, deviceWithID.LastSeenIP, gotDevice.LastSeenIP)
		assert.Greater(t, gotDevice.LastSeenTS, updatedAfterTimestamp)

		// create one more device and remove the devices step by step
		newDeviceID := util.RandomString(16)
		accessToken = util.RandomString(16)
		_, err = db.CreateDevice(ctx, localpart, domain, &newDeviceID, accessToken, nil, "", "")
		assert.NoError(t, err, "unable to create new device")

		devices, err = db.GetDevicesByLocalpart(ctx, localpart, domain)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, 3, len(devices))

		err = db.RemoveDevices(ctx, localpart, domain, deviceIDs)
		assert.NoError(t, err, "unable to remove devices")
		devices, err = db.GetDevicesByLocalpart(ctx, localpart, domain)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, 1, len(devices))

		deleted, err := db.RemoveAllDevices(ctx, localpart, domain, "")
		assert.NoError(t, err, "unable to remove all devices")
		assert.Equal(t, 1, len(deleted))
		assert.Equal(t, newDeviceID, deleted[0].ID)
	})
}

func Test_KeyBackup(t *testing.T) {
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		wantAuthData := json.RawMessage("my auth data")
		wantVersion, err := db.CreateKeyBackup(ctx, alice.ID, "dummyAlgo", wantAuthData)
		assert.NoError(t, err, "unable to create key backup")
		// get key backup by version
		gotVersion, gotAlgo, gotAuthData, _, _, err := db.GetKeyBackup(ctx, alice.ID, wantVersion)
		assert.NoError(t, err, "unable to get key backup")
		assert.Equal(t, wantVersion, gotVersion, "backup version mismatch")
		assert.Equal(t, "dummyAlgo", gotAlgo, "backup algorithm mismatch")
		assert.Equal(t, wantAuthData, gotAuthData, "backup auth data mismatch")

		// get any key backup
		gotVersion, gotAlgo, gotAuthData, _, _, err = db.GetKeyBackup(ctx, alice.ID, "")
		assert.NoError(t, err, "unable to get key backup")
		assert.Equal(t, wantVersion, gotVersion, "backup version mismatch")
		assert.Equal(t, "dummyAlgo", gotAlgo, "backup algorithm mismatch")
		assert.Equal(t, wantAuthData, gotAuthData, "backup auth data mismatch")

		err = db.UpdateKeyBackupAuthData(ctx, alice.ID, wantVersion, json.RawMessage("my updated auth data"))
		assert.NoError(t, err, "unable to update key backup auth data")

		uploads := []api.InternalKeyBackupSession{
			{
				KeyBackupSession: api.KeyBackupSession{
					IsVerified:  true,
					SessionData: wantAuthData,
				},
				RoomID:    room.ID,
				SessionID: "1",
			},
			{
				KeyBackupSession: api.KeyBackupSession{},
				RoomID:           room.ID,
				SessionID:        "2",
			},
		}
		count, _, err := db.UpsertBackupKeys(ctx, wantVersion, alice.ID, uploads)
		assert.NoError(t, err, "unable to upsert backup keys")
		assert.Equal(t, int64(len(uploads)), count, "unexpected backup count")

		// do it again to update a key
		uploads[1].IsVerified = true
		count, _, err = db.UpsertBackupKeys(ctx, wantVersion, alice.ID, uploads[1:])
		assert.NoError(t, err, "unable to upsert backup keys")
		assert.Equal(t, int64(len(uploads)), count, "unexpected backup count")

		// get backup keys by session id
		gotBackupKeys, err := db.GetBackupKeys(ctx, wantVersion, alice.ID, room.ID, "1")
		assert.NoError(t, err, "unable to get backup keys")
		assert.Equal(t, uploads[0].KeyBackupSession, gotBackupKeys[room.ID]["1"])

		// get backup keys by room id
		gotBackupKeys, err = db.GetBackupKeys(ctx, wantVersion, alice.ID, room.ID, "")
		assert.NoError(t, err, "unable to get backup keys")
		assert.Equal(t, uploads[0].KeyBackupSession, gotBackupKeys[room.ID]["1"])

		gotCount, err := db.CountBackupKeys(ctx, wantVersion, alice.ID)
		assert.NoError(t, err, "unable to get backup keys count")
		assert.Equal(t, count, gotCount, "unexpected backup count")

		// finally delete a key
		exists, err := db.DeleteKeyBackup(ctx, alice.ID, wantVersion)
		assert.NoError(t, err, "unable to delete key backup")
		assert.True(t, exists)

		// this key should not exist
		exists, err = db.DeleteKeyBackup(ctx, alice.ID, "3")
		assert.NoError(t, err, "unable to delete key backup")
		assert.False(t, exists)
	})
}

func Test_LoginToken(t *testing.T) {
	alice := test.NewUser(t)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		// create a new token
		wantLoginToken := &api.LoginTokenData{UserID: alice.ID}

		gotMetadata, err := db.CreateLoginToken(ctx, wantLoginToken)
		assert.NoError(t, err, "unable to create login token")
		assert.NotNil(t, gotMetadata)
		assert.Equal(t, time.Now().Add(loginTokenLifetime).Truncate(loginTokenLifetime), gotMetadata.Expiration.Truncate(loginTokenLifetime))

		// get the new token
		gotLoginToken, err := db.GetLoginTokenDataByToken(ctx, gotMetadata.Token)
		assert.NoError(t, err, "unable to get login token")
		assert.NotNil(t, gotLoginToken)
		assert.Equal(t, wantLoginToken, gotLoginToken, "unexpected login token")

		// remove the login token again
		err = db.RemoveLoginToken(ctx, gotMetadata.Token)
		assert.NoError(t, err, "unable to remove login token")

		// check if the token was actually deleted
		_, err = db.GetLoginTokenDataByToken(ctx, gotMetadata.Token)
		assert.Error(t, err, "expected an error, but got none")
	})
}

func Test_OpenID(t *testing.T) {
	alice := test.NewUser(t)
	token := util.RandomString(24)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		expiresAtMS := time.Now().UnixNano()/int64(time.Millisecond) + openIDLifetimeMS
		expires, err := db.CreateOpenIDToken(ctx, token, alice.ID)
		assert.NoError(t, err, "unable to create OpenID token")
		assert.Equal(t, expiresAtMS, expires)

		attributes, err := db.GetOpenIDTokenAttributes(ctx, token)
		assert.NoError(t, err, "unable to get OpenID token attributes")
		assert.Equal(t, alice.ID, attributes.UserID)
		assert.Equal(t, expiresAtMS, attributes.ExpiresAtMS)
	})
}

func Test_Profile(t *testing.T) {
	alice := test.NewUser(t)
	aliceLocalpart, aliceDomain, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		// create account, which also creates a profile
		_, err = db.CreateAccount(ctx, aliceLocalpart, aliceDomain, "testing", "", api.AccountTypeAdmin)
		assert.NoError(t, err, "failed to create account")

		gotProfile, err := db.GetProfileByLocalpart(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to get profile by localpart")
		wantProfile := &authtypes.Profile{
			Localpart:  aliceLocalpart,
			ServerName: string(aliceDomain),
		}
		assert.Equal(t, wantProfile, gotProfile)

		// set avatar & displayname
		wantProfile.DisplayName = "Alice"
		gotProfile, changed, err := db.SetDisplayName(ctx, aliceLocalpart, aliceDomain, "Alice")
		assert.Equal(t, wantProfile, gotProfile)
		assert.NoError(t, err, "unable to set displayname")
		assert.True(t, changed)

		wantProfile.AvatarURL = "mxc://aliceAvatar"
		gotProfile, changed, err = db.SetAvatarURL(ctx, aliceLocalpart, aliceDomain, "mxc://aliceAvatar")
		assert.NoError(t, err, "unable to set avatar url")
		assert.Equal(t, wantProfile, gotProfile)
		assert.True(t, changed)

		// Setting the same avatar again doesn't change anything
		wantProfile.AvatarURL = "mxc://aliceAvatar"
		gotProfile, changed, err = db.SetAvatarURL(ctx, aliceLocalpart, aliceDomain, "mxc://aliceAvatar")
		assert.NoError(t, err, "unable to set avatar url")
		assert.Equal(t, wantProfile, gotProfile)
		assert.False(t, changed)

		// search profiles
		searchRes, err := db.SearchProfiles(ctx, "Alice", 2)
		assert.NoError(t, err, "unable to search profiles")
		assert.Equal(t, 1, len(searchRes))
		assert.Equal(t, *wantProfile, searchRes[0])
	})
}

func Test_Pusher(t *testing.T) {
	alice := test.NewUser(t)
	aliceLocalpart, aliceDomain, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		appID := util.RandomString(8)
		var pushKeys []string
		var gotPushers []api.Pusher
		for i := 0; i < 2; i++ {
			pushKey := util.RandomString(8)

			wantPusher := api.Pusher{
				PushKey:           pushKey,
				Kind:              api.HTTPKind,
				AppID:             appID,
				AppDisplayName:    util.RandomString(8),
				DeviceDisplayName: util.RandomString(8),
				ProfileTag:        util.RandomString(8),
				Language:          util.RandomString(2),
			}
			err = db.UpsertPusher(ctx, wantPusher, aliceLocalpart, aliceDomain)
			assert.NoError(t, err, "unable to upsert pusher")

			// check it was actually persisted
			gotPushers, err = db.GetPushers(ctx, aliceLocalpart, aliceDomain)
			assert.NoError(t, err, "unable to get pushers")
			assert.Equal(t, i+1, len(gotPushers))
			assert.Equal(t, wantPusher, gotPushers[i])
			pushKeys = append(pushKeys, pushKey)
		}

		// remove single pusher
		err = db.RemovePusher(ctx, appID, pushKeys[0], aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to remove pusher")
		gotPushers, err := db.GetPushers(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to get pushers")
		assert.Equal(t, 1, len(gotPushers))

		// remove last pusher
		err = db.RemovePushers(ctx, appID, pushKeys[1])
		assert.NoError(t, err, "unable to remove pusher")
		gotPushers, err = db.GetPushers(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to get pushers")
		assert.Equal(t, 0, len(gotPushers))
	})
}

func Test_ThreePID(t *testing.T) {
	alice := test.NewUser(t)
	aliceLocalpart, aliceDomain, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		threePID := util.RandomString(8)
		medium := util.RandomString(8)
		err = db.SaveThreePIDAssociation(ctx, threePID, aliceLocalpart, aliceDomain, medium)
		assert.NoError(t, err, "unable to save threepid association")

		// get the stored threepid
		gotLocalpart, gotDomain, err := db.GetLocalpartForThreePID(ctx, threePID, medium)
		assert.NoError(t, err, "unable to get localpart for threepid")
		assert.Equal(t, aliceLocalpart, gotLocalpart)
		assert.Equal(t, aliceDomain, gotDomain)

		threepids, err := db.GetThreePIDsForLocalpart(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to get threepids for localpart")
		assert.Equal(t, 1, len(threepids))
		assert.Equal(t, authtypes.ThreePID{
			Address: threePID,
			Medium:  medium,
		}, threepids[0])

		// remove threepid association
		err = db.RemoveThreePIDAssociation(ctx, threePID, medium)
		assert.NoError(t, err, "unexpected error")

		// verify it was deleted
		threepids, err = db.GetThreePIDsForLocalpart(ctx, aliceLocalpart, aliceDomain)
		assert.NoError(t, err, "unable to get threepids for localpart")
		assert.Equal(t, 0, len(threepids))
	})
}

func Test_Notification(t *testing.T) {
	alice := test.NewUser(t)
	aliceLocalpart, aliceDomain, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)
	room := test.NewRoom(t, alice)
	room2 := test.NewRoom(t, alice)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		// generate some dummy notifications
		for i := 0; i < 10; i++ {
			eventID := util.RandomString(16)
			roomID := room.ID
			ts := time.Now()
			if i > 5 {
				roomID = room2.ID
				// create some old notifications to test DeleteOldNotifications
				ts = ts.AddDate(0, -2, 0)
			}
			notification := &api.Notification{
				Actions: []*pushrules.Action{
					{},
				},
				Event: gomatrixserverlib.ClientEvent{
					Content: gomatrixserverlib.RawJSON("{}"),
				},
				Read:   false,
				RoomID: roomID,
				TS:     gomatrixserverlib.AsTimestamp(ts),
			}
			err = db.InsertNotification(ctx, aliceLocalpart, aliceDomain, eventID, uint64(i+1), nil, notification)
			assert.NoError(t, err, "unable to insert notification")
		}

		// get notifications
		count, err := db.GetNotificationCount(ctx, aliceLocalpart, aliceDomain, tables.AllNotifications)
		assert.NoError(t, err, "unable to get notification count")
		assert.Equal(t, int64(10), count)
		notifs, count, err := db.GetNotifications(ctx, aliceLocalpart, aliceDomain, 0, 15, tables.AllNotifications)
		assert.NoError(t, err, "unable to get notifications")
		assert.Equal(t, int64(10), count)
		assert.Equal(t, 10, len(notifs))
		// ... for a specific room
		total, _, err := db.GetRoomNotificationCounts(ctx, aliceLocalpart, aliceDomain, room2.ID)
		assert.NoError(t, err, "unable to get notifications for room")
		assert.Equal(t, int64(4), total)

		// mark notification as read
		affected, err := db.SetNotificationsRead(ctx, aliceLocalpart, aliceDomain, room2.ID, 7, true)
		assert.NoError(t, err, "unable to set notifications read")
		assert.True(t, affected)

		// this should delete 2 notifications
		affected, err = db.DeleteNotificationsUpTo(ctx, aliceLocalpart, aliceDomain, room2.ID, 8)
		assert.NoError(t, err, "unable to set notifications read")
		assert.True(t, affected)

		total, _, err = db.GetRoomNotificationCounts(ctx, aliceLocalpart, aliceDomain, room2.ID)
		assert.NoError(t, err, "unable to get notifications for room")
		assert.Equal(t, int64(2), total)

		// delete old notifications
		err = db.DeleteOldNotifications(ctx)
		assert.NoError(t, err)

		// this should now return 0 notifications
		total, _, err = db.GetRoomNotificationCounts(ctx, aliceLocalpart, aliceDomain, room2.ID)
		assert.NoError(t, err, "unable to get notifications for room")
		assert.Equal(t, int64(0), total)
	})
}
