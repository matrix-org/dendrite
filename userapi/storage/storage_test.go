package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

const loginTokenLifetime = time.Minute

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := storage.NewUserAPIDatabase(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, "localhost", bcrypt.MinCost, time.Minute.Milliseconds(), loginTokenLifetime, "_server")
	if err != nil {
		t.Fatalf("NewUserAPIDatabase returned %s", err)
	}
	return db, close
}

// Tests storing and getting account data
func Test_AccountData(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		alice := test.NewUser()
		localpart, _, err := gomatrixserverlib.SplitID('@', alice.ID)
		assert.NoError(t, err)

		room := test.NewRoom(t, alice)
		events := room.Events()

		contentRoom := json.RawMessage(fmt.Sprintf(`{"event_id":"%s"}`, events[len(events)-1].EventID()))
		err = db.SaveAccountData(ctx, localpart, room.ID, "m.fully_read", contentRoom)
		assert.NoError(t, err, "unable to save account data")

		contentGlobal := json.RawMessage(fmt.Sprintf(`{"recent_rooms":["%s"]}`, room.ID))
		err = db.SaveAccountData(ctx, localpart, "", "im.vector.setting.breadcrumbs", contentGlobal)
		assert.NoError(t, err, "unable to save account data")

		accountData, err := db.GetAccountDataByType(ctx, localpart, room.ID, "m.fully_read")
		assert.NoError(t, err, "unable to get account data by type")
		assert.Equal(t, contentRoom, accountData)

		globalData, roomData, err := db.GetAccountData(ctx, localpart)
		assert.NoError(t, err)
		assert.Equal(t, contentRoom, roomData[room.ID]["m.fully_read"])
		assert.Equal(t, contentGlobal, globalData["im.vector.setting.breadcrumbs"])
	})
}

// Tests the creation of accounts
func Test_Accounts(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		_ = close
		alice := test.NewUser()
		aliceLocalpart, _, err := gomatrixserverlib.SplitID('@', alice.ID)
		assert.NoError(t, err)

		accAlice, err := db.CreateAccount(ctx, aliceLocalpart, "testing", "", api.AccountTypeAdmin)
		assert.NoError(t, err, "failed to create account")
		// verify the newly create account is the same as returned by CreateAccount
		var accGet *api.Account
		accGet, err = db.GetAccountByPassword(ctx, aliceLocalpart, "testing")
		assert.NoError(t, err, "failed to get account by password")
		assert.Equal(t, accAlice, accGet)
		accGet, err = db.GetAccountByLocalpart(ctx, aliceLocalpart)
		assert.NoError(t, err, "failed to get account by localpart")
		assert.Equal(t, accAlice, accGet)

		// check account availability
		available, err := db.CheckAccountAvailability(ctx, aliceLocalpart)
		assert.NoError(t, err, "failed to checkout account availability")
		assert.Equal(t, false, available)

		available, err = db.CheckAccountAvailability(ctx, "unusedname")
		assert.NoError(t, err, "failed to checkout account availability")
		assert.Equal(t, true, available)

		// get guest account numeric aliceLocalpart
		first, err := db.GetNewNumericLocalpart(ctx)
		assert.NoError(t, err, "failed to get new numeric localpart")
		// SQLite requires a new user to be created, as it doesn't have a sequence and uses the count(localpart) instead
		_, err = db.CreateAccount(ctx, strconv.Itoa(int(first)), "testing", "", api.AccountTypeAdmin)
		assert.NoError(t, err, "failed to create account")
		second, err := db.GetNewNumericLocalpart(ctx)
		assert.NoError(t, err)
		assert.Greater(t, second, first)

		// update password for alice
		err = db.SetPassword(ctx, aliceLocalpart, "newPassword")
		assert.NoError(t, err, "failed to update password")
		accGet, err = db.GetAccountByPassword(ctx, aliceLocalpart, "newPassword")
		assert.NoError(t, err, "failed to get account by new password")
		assert.Equal(t, accAlice, accGet)

		// deactivate account
		err = db.DeactivateAccount(ctx, aliceLocalpart)
		assert.NoError(t, err, "failed to deactivate account")
		// This should fail now, as the account is deactivated
		_, err = db.GetAccountByPassword(ctx, aliceLocalpart, "newPassword")
		assert.Error(t, err, "expected an error, got none")

		_, err = db.GetAccountByLocalpart(ctx, "unusename")
		assert.Error(t, err, "expected an error for non existent localpart")
	})
}

func Test_Devices(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser()
	localpart, _, err := gomatrixserverlib.SplitID('@', alice.ID)
	assert.NoError(t, err)
	deviceID := util.RandomString(8)
	accessToken := util.RandomString(16)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()

		deviceWithID, err := db.CreateDevice(ctx, localpart, &deviceID, accessToken, nil, "", "")
		assert.NoError(t, err, "unable to create deviceWithoutID")

		gotDevice, err := db.GetDeviceByID(ctx, localpart, deviceID)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, deviceWithID.ID, gotDevice.ID) // GetDeviceByID doesn't populate all fields

		gotDeviceAccessToken, err := db.GetDeviceByAccessToken(ctx, accessToken)
		assert.NoError(t, err, "unable to get device by access token")
		assert.Equal(t, deviceWithID.ID, gotDeviceAccessToken.ID) // GetDeviceByAccessToken doesn't populate all fields

		// create a device without existing device ID
		accessToken = util.RandomString(16)
		deviceWithoutID, err := db.CreateDevice(ctx, localpart, nil, accessToken, nil, "", "")
		assert.NoError(t, err, "unable to create deviceWithoutID")
		gotDeviceWithoutID, err := db.GetDeviceByID(ctx, localpart, deviceWithoutID.ID)
		assert.NoError(t, err, "unable to get device by id")
		assert.Equal(t, deviceWithoutID.ID, gotDeviceWithoutID.ID) // GetDeviceByID doesn't populate all fields

		// Get devices
		devices, err := db.GetDevicesByLocalpart(ctx, localpart)
		assert.NoError(t, err, "unable to get devices by localpart")
		deviceIDs := make([]string, 0, len(devices))
		for _, dev := range devices {
			deviceIDs = append(deviceIDs, dev.ID)
		}

		devices2, err := db.GetDevicesByID(ctx, deviceIDs)
		assert.NoError(t, err, "unable to get devices by id")
		assert.Equal(t, devices, devices2)
	})
}

func Test_KeyBackup(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser()
	//localpart, _, err := gomatrixserverlib.SplitID('@', alice.ID)
	//assert.NoError(t, err)
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
	ctx := context.Background()
	alice := test.NewUser()
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
