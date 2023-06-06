package shared_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
	ed255192 "golang.org/x/crypto/ed25519"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/shared"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateRoomserverDatabase(t *testing.T, dbType test.DBType) (*shared.Database, func()) {
	t.Helper()

	connStr, clearDB := test.PrepareDBConnectionString(t, dbType)
	dbOpts := &config.DatabaseOptions{ConnectionString: config.DataSource(connStr)}

	writer := sqlutil.NewExclusiveWriter()
	db, err := sqlutil.Open(dbOpts, writer)
	assert.NoError(t, err)

	var membershipTable tables.Membership
	var stateKeyTable tables.EventStateKeys
	var userRoomKeys tables.UserRoomKeys
	var roomsTable tables.Rooms
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateRoomsTable(db)
		assert.NoError(t, err)
		err = postgres.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		err = postgres.CreateMembershipTable(db)
		assert.NoError(t, err)
		err = postgres.CreateUserRoomKeysTable(db)
		assert.NoError(t, err)
		roomsTable, err = postgres.PrepareRoomsTable(db)
		assert.NoError(t, err)
		membershipTable, err = postgres.PrepareMembershipTable(db)
		assert.NoError(t, err)
		stateKeyTable, err = postgres.PrepareEventStateKeysTable(db)
		assert.NoError(t, err)
		userRoomKeys, err = postgres.PrepareUserRoomKeysTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateRoomsTable(db)
		assert.NoError(t, err)
		err = sqlite3.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		err = sqlite3.CreateMembershipTable(db)
		assert.NoError(t, err)
		err = sqlite3.CreateUserRoomKeysTable(db)
		assert.NoError(t, err)
		roomsTable, err = sqlite3.PrepareRoomsTable(db)
		assert.NoError(t, err)
		membershipTable, err = sqlite3.PrepareMembershipTable(db)
		assert.NoError(t, err)
		stateKeyTable, err = sqlite3.PrepareEventStateKeysTable(db)
		assert.NoError(t, err)
		userRoomKeys, err = sqlite3.PrepareUserRoomKeysTable(db)
	}
	assert.NoError(t, err)

	cache := caching.NewRistrettoCache(8*1024*1024, time.Hour, false)

	evDb := shared.EventDatabase{EventStateKeysTable: stateKeyTable, Cache: cache, Writer: writer}

	return &shared.Database{
			DB:               db,
			EventDatabase:    evDb,
			MembershipTable:  membershipTable,
			UserRoomKeyTable: userRoomKeys,
			RoomsTable:       roomsTable,
			Writer:           writer,
			Cache:            cache,
		}, func() {
			clearDB()
			err = db.Close()
			assert.NoError(t, err)
		}
}

func Test_GetLeftUsers(t *testing.T) {
	alice := test.NewUser(t)
	bob := test.NewUser(t)
	charlie := test.NewUser(t)

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRoomserverDatabase(t, dbType)
		defer close()

		// Create dummy entries
		for _, user := range []*test.User{alice, bob, charlie} {
			nid, err := db.EventStateKeysTable.InsertEventStateKeyNID(ctx, nil, user.ID)
			assert.NoError(t, err)
			err = db.MembershipTable.InsertMembership(ctx, nil, 1, nid, true)
			assert.NoError(t, err)
			// We must update the membership with a non-zero event NID or it will get filtered out in later queries
			membershipNID := tables.MembershipStateLeaveOrBan
			if user == alice {
				membershipNID = tables.MembershipStateJoin
			}
			_, err = db.MembershipTable.UpdateMembership(ctx, nil, 1, nid, nid, membershipNID, 1, false)
			assert.NoError(t, err)
		}

		// Now try to get the left users, this should be Bob and Charlie, since they have a "leave" membership
		expectedUserIDs := []string{bob.ID, charlie.ID}
		leftUsers, err := db.GetLeftUsers(context.Background(), []string{alice.ID, bob.ID, charlie.ID})
		assert.NoError(t, err)
		assert.ElementsMatch(t, expectedUserIDs, leftUsers)
	})
}

func TestUserRoomKeys(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	userID, err := spec.NewUserID(alice.ID, true)
	assert.NoError(t, err)
	roomID, err := spec.NewRoomID(room.ID)
	assert.NoError(t, err)

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRoomserverDatabase(t, dbType)
		defer close()

		// create a room NID so we can query the room
		_, err = db.RoomsTable.InsertRoomNID(ctx, nil, roomID.String(), gomatrixserverlib.RoomVersionV10)
		assert.NoError(t, err)
		doesNotExist, err := spec.NewRoomID("!doesnotexist:localhost")
		assert.NoError(t, err)
		_, err = db.RoomsTable.InsertRoomNID(ctx, nil, doesNotExist.String(), gomatrixserverlib.RoomVersionV10)
		assert.NoError(t, err)

		_, key, err := ed25519.GenerateKey(nil)
		assert.NoError(t, err)

		gotKey, err := db.InsertUserRoomPrivateKey(ctx, *userID, *roomID, key)
		assert.NoError(t, err)
		assert.Equal(t, gotKey, key)

		// again, this shouldn't result in an error, but return the existing key
		_, key2, err := ed25519.GenerateKey(nil)
		assert.NoError(t, err)
		gotKey, err = db.InsertUserRoomPrivateKey(ctx, *userID, *roomID, key2)
		assert.NoError(t, err)
		assert.Equal(t, gotKey, key)

		gotKey, err = db.SelectUserRoomPrivateKey(context.Background(), *userID, *roomID)
		assert.NoError(t, err)
		assert.Equal(t, key, gotKey)

		// Key doesn't exist, we shouldn't get anything back
		assert.NoError(t, err)
		gotKey, err = db.SelectUserRoomPrivateKey(context.Background(), *userID, *doesNotExist)
		assert.NoError(t, err)
		assert.Nil(t, gotKey)

		userIDs, err := db.SelectUserIDsForPublicKeys(ctx, [][]byte{key.Public().(ed25519.PublicKey)})
		assert.NoError(t, err)
		assert.NotNil(t, userIDs)

		// insert key that came in over federation
		var gotPublicKey, key4 ed255192.PublicKey
		key4, _, err = ed25519.GenerateKey(nil)
		assert.NoError(t, err)
		gotPublicKey, err = db.InsertUserRoomPublicKey(context.Background(), *userID, *doesNotExist, key4)
		assert.NoError(t, err)
		assert.Equal(t, key4, gotPublicKey)

		// test invalid room
		reallyDoesNotExist, err := spec.NewRoomID("!reallydoesnotexist:localhost")
		assert.NoError(t, err)
		_, err = db.InsertUserRoomPublicKey(context.Background(), *userID, *reallyDoesNotExist, key4)
		assert.Error(t, err)
		_, err = db.InsertUserRoomPrivateKey(context.Background(), *userID, *reallyDoesNotExist, key)
		assert.Error(t, err)
	})
}
