package storage_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/keyserver/storage"
	"github.com/matrix-org/dendrite/keyserver/types"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
)

var ctx = context.Background()

func MustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	base, close := testrig.CreateBaseDendrite(t, dbType)
	db, err := storage.NewDatabase(base, &base.Cfg.KeyServer.Database)
	if err != nil {
		t.Fatalf("failed to create new database: %v", err)
	}
	return db, close
}

func MustNotError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatalf("operation failed: %s", err)
}

func TestKeyChanges(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, clean := MustCreateDatabase(t, dbType)
		defer clean()
		_, err := db.StoreKeyChange(ctx, "@alice:localhost")
		MustNotError(t, err)
		deviceChangeIDB, err := db.StoreKeyChange(ctx, "@bob:localhost")
		MustNotError(t, err)
		deviceChangeIDC, err := db.StoreKeyChange(ctx, "@charlie:localhost")
		MustNotError(t, err)
		userIDs, latest, err := db.KeyChanges(ctx, deviceChangeIDB, types.OffsetNewest)
		if err != nil {
			t.Fatalf("Failed to KeyChanges: %s", err)
		}
		if latest != deviceChangeIDC {
			t.Fatalf("KeyChanges: got latest=%d want %d", latest, deviceChangeIDC)
		}
		if !reflect.DeepEqual(userIDs, []string{"@charlie:localhost"}) {
			t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
		}
	})
}

func TestKeyChangesNoDupes(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, clean := MustCreateDatabase(t, dbType)
		defer clean()
		deviceChangeIDA, err := db.StoreKeyChange(ctx, "@alice:localhost")
		MustNotError(t, err)
		deviceChangeIDB, err := db.StoreKeyChange(ctx, "@alice:localhost")
		MustNotError(t, err)
		if deviceChangeIDA == deviceChangeIDB {
			t.Fatalf("Expected change ID to be different even when inserting key change for the same user, got %d for both changes", deviceChangeIDA)
		}
		deviceChangeID, err := db.StoreKeyChange(ctx, "@alice:localhost")
		MustNotError(t, err)
		userIDs, latest, err := db.KeyChanges(ctx, 0, types.OffsetNewest)
		if err != nil {
			t.Fatalf("Failed to KeyChanges: %s", err)
		}
		if latest != deviceChangeID {
			t.Fatalf("KeyChanges: got latest=%d want %d", latest, deviceChangeID)
		}
		if !reflect.DeepEqual(userIDs, []string{"@alice:localhost"}) {
			t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
		}
	})
}

func TestKeyChangesUpperLimit(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, clean := MustCreateDatabase(t, dbType)
		defer clean()
		deviceChangeIDA, err := db.StoreKeyChange(ctx, "@alice:localhost")
		MustNotError(t, err)
		deviceChangeIDB, err := db.StoreKeyChange(ctx, "@bob:localhost")
		MustNotError(t, err)
		_, err = db.StoreKeyChange(ctx, "@charlie:localhost")
		MustNotError(t, err)
		userIDs, latest, err := db.KeyChanges(ctx, deviceChangeIDA, deviceChangeIDB)
		if err != nil {
			t.Fatalf("Failed to KeyChanges: %s", err)
		}
		if latest != deviceChangeIDB {
			t.Fatalf("KeyChanges: got latest=%d want %d", latest, deviceChangeIDB)
		}
		if !reflect.DeepEqual(userIDs, []string{"@bob:localhost"}) {
			t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
		}
	})
}

var dbLock sync.Mutex
var deviceArray = []string{"AAA", "another_device"}

// The purpose of this test is to make sure that the storage layer is generating sequential stream IDs per user,
// and that they are returned correctly when querying for device keys.
func TestDeviceKeysStreamIDGeneration(t *testing.T) {
	var err error
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, clean := MustCreateDatabase(t, dbType)
		defer clean()
		alice := "@alice:TestDeviceKeysStreamIDGeneration"
		bob := "@bob:TestDeviceKeysStreamIDGeneration"
		msgs := []api.DeviceMessage{
			{
				Type: api.TypeDeviceKeyUpdate,
				DeviceKeys: &api.DeviceKeys{
					DeviceID: "AAA",
					UserID:   alice,
					KeyJSON:  []byte(`{"key":"v1"}`),
				},
				// StreamID: 1
			},
			{
				Type: api.TypeDeviceKeyUpdate,
				DeviceKeys: &api.DeviceKeys{
					DeviceID: "AAA",
					UserID:   bob,
					KeyJSON:  []byte(`{"key":"v1"}`),
				},
				// StreamID: 1 as this is a different user
			},
			{
				Type: api.TypeDeviceKeyUpdate,
				DeviceKeys: &api.DeviceKeys{
					DeviceID: "another_device",
					UserID:   alice,
					KeyJSON:  []byte(`{"key":"v1"}`),
				},
				// StreamID: 2 as this is a 2nd device key
			},
		}
		MustNotError(t, db.StoreLocalDeviceKeys(ctx, msgs))
		if msgs[0].StreamID != 1 {
			t.Fatalf("Expected StoreLocalDeviceKeys to set StreamID=1 but got %d", msgs[0].StreamID)
		}
		if msgs[1].StreamID != 1 {
			t.Fatalf("Expected StoreLocalDeviceKeys to set StreamID=1 (different user) but got %d", msgs[1].StreamID)
		}
		if msgs[2].StreamID != 2 {
			t.Fatalf("Expected StoreLocalDeviceKeys to set StreamID=2 (another device) but got %d", msgs[2].StreamID)
		}

		// updating a device sets the next stream ID for that user
		msgs = []api.DeviceMessage{
			{
				Type: api.TypeDeviceKeyUpdate,
				DeviceKeys: &api.DeviceKeys{
					DeviceID: "AAA",
					UserID:   alice,
					KeyJSON:  []byte(`{"key":"v2"}`),
				},
				// StreamID: 3
			},
		}
		MustNotError(t, db.StoreLocalDeviceKeys(ctx, msgs))
		if msgs[0].StreamID != 3 {
			t.Fatalf("Expected StoreLocalDeviceKeys to set StreamID=3 (new key same device) but got %d", msgs[0].StreamID)
		}

		dbLock.Lock()
		defer dbLock.Unlock()
		// Querying for device keys returns the latest stream IDs
		msgs, err = db.DeviceKeysForUser(ctx, alice, deviceArray, false)

		if err != nil {
			t.Fatalf("DeviceKeysForUser returned error: %s", err)
		}
		wantStreamIDs := map[string]int64{
			"AAA":            3,
			"another_device": 2,
		}
		if len(msgs) != len(wantStreamIDs) {
			t.Fatalf("DeviceKeysForUser: wrong number of devices, got %d want %d", len(msgs), len(wantStreamIDs))
		}
		for _, m := range msgs {
			if m.StreamID != wantStreamIDs[m.DeviceID] {
				t.Errorf("DeviceKeysForUser: wrong returned stream ID for key, got %d want %d", m.StreamID, wantStreamIDs[m.DeviceID])
			}
		}
	})
}
