package tables_test

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"testing"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage/postgres"
	"github.com/element-hq/dendrite/roomserver/storage/sqlite3"
	"github.com/element-hq/dendrite/roomserver/storage/tables"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
	ed255192 "golang.org/x/crypto/ed25519"
)

func mustCreateUserRoomKeysTable(t *testing.T, dbType test.DBType) (tab tables.UserRoomKeys, db *sql.DB, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateUserRoomKeysTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareUserRoomKeysTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateUserRoomKeysTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareUserRoomKeysTable(db)
	}
	assert.NoError(t, err)

	return tab, db, close
}

func TestUserRoomKeysTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, db, close := mustCreateUserRoomKeysTable(t, dbType)
		defer close()
		userNID := types.EventStateKeyNID(1)
		roomNID := types.RoomNID(1)
		_, key, err := ed25519.GenerateKey(nil)
		assert.NoError(t, err)

		err = sqlutil.WithTransaction(db, func(txn *sql.Tx) error {
			var gotKey, key2, key3 ed25519.PrivateKey
			var pubKey ed25519.PublicKey
			gotKey, err = tab.InsertUserRoomPrivatePublicKey(context.Background(), txn, userNID, roomNID, key)
			assert.NoError(t, err)
			assert.Equal(t, gotKey, key)

			// again, this shouldn't result in an error, but return the existing key
			_, key2, err = ed25519.GenerateKey(nil)
			assert.NoError(t, err)
			gotKey, err = tab.InsertUserRoomPrivatePublicKey(context.Background(), txn, userNID, roomNID, key2)
			assert.NoError(t, err)
			assert.Equal(t, gotKey, key)

			// add another user
			_, key3, err = ed25519.GenerateKey(nil)
			assert.NoError(t, err)
			userNID2 := types.EventStateKeyNID(2)
			_, err = tab.InsertUserRoomPrivatePublicKey(context.Background(), txn, userNID2, roomNID, key3)
			assert.NoError(t, err)

			gotKey, err = tab.SelectUserRoomPrivateKey(context.Background(), txn, userNID, roomNID)
			assert.NoError(t, err)
			assert.Equal(t, key, gotKey)
			pubKey, err = tab.SelectUserRoomPublicKey(context.Background(), txn, userNID, roomNID)
			assert.NoError(t, err)
			assert.Equal(t, key.Public(), pubKey)

			// try to update an existing key, this should only be done for users NOT on this homeserver
			var gotPubKey ed25519.PublicKey
			gotPubKey, err = tab.InsertUserRoomPublicKey(context.Background(), txn, userNID, roomNID, key2.Public().(ed25519.PublicKey))
			assert.NoError(t, err)
			assert.Equal(t, key2.Public(), gotPubKey)

			// Key doesn't exist
			gotKey, err = tab.SelectUserRoomPrivateKey(context.Background(), txn, userNID, 2)
			assert.NoError(t, err)
			assert.Nil(t, gotKey)
			pubKey, err = tab.SelectUserRoomPublicKey(context.Background(), txn, userNID, 2)
			assert.NoError(t, err)
			assert.Nil(t, pubKey)

			// query user NIDs for senderKeys
			var gotKeys map[string]types.UserRoomKeyPair
			query := map[types.RoomNID][]ed25519.PublicKey{
				roomNID:          {key2.Public().(ed25519.PublicKey), key3.Public().(ed25519.PublicKey)},
				types.RoomNID(2): {key.Public().(ed25519.PublicKey), key3.Public().(ed25519.PublicKey)}, // doesn't exist
			}
			gotKeys, err = tab.BulkSelectUserNIDs(context.Background(), txn, query)
			assert.NoError(t, err)
			assert.NotNil(t, gotKeys)

			wantKeys := map[string]types.UserRoomKeyPair{
				string(spec.Base64Bytes(key2.Public().(ed25519.PublicKey)).Encode()): {RoomNID: roomNID, EventStateKeyNID: userNID},
				string(spec.Base64Bytes(key3.Public().(ed25519.PublicKey)).Encode()): {RoomNID: roomNID, EventStateKeyNID: userNID2},
			}
			assert.Equal(t, wantKeys, gotKeys)

			// insert key that came in over federation
			var gotPublicKey, key4 ed255192.PublicKey
			key4, _, err = ed25519.GenerateKey(nil)
			assert.NoError(t, err)
			gotPublicKey, err = tab.InsertUserRoomPublicKey(context.Background(), txn, userNID, 2, key4)
			assert.NoError(t, err)
			assert.Equal(t, key4, gotPublicKey)

			return nil
		})
		assert.NoError(t, err)

	})
}
