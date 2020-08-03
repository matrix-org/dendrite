package storage

import (
	"context"
	"reflect"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/matrix-org/dendrite/keyserver/api"
)

var ctx = context.Background()

func MustNotError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatalf("operation failed: %s", err)
}

func TestKeyChanges(t *testing.T) {
	db, err := NewDatabase("file::memory:", nil)
	if err != nil {
		t.Fatalf("Failed to NewDatabase: %s", err)
	}
	MustNotError(t, db.StoreKeyChange(ctx, 0, 0, "@alice:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 1, "@bob:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 2, "@charlie:localhost"))
	userIDs, latest, err := db.KeyChanges(ctx, 0, 1, sarama.OffsetNewest)
	if err != nil {
		t.Fatalf("Failed to KeyChanges: %s", err)
	}
	if latest != 2 {
		t.Fatalf("KeyChanges: got latest=%d want 2", latest)
	}
	if !reflect.DeepEqual(userIDs, []string{"@charlie:localhost"}) {
		t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
	}
}

func TestKeyChangesNoDupes(t *testing.T) {
	db, err := NewDatabase("file::memory:", nil)
	if err != nil {
		t.Fatalf("Failed to NewDatabase: %s", err)
	}
	MustNotError(t, db.StoreKeyChange(ctx, 0, 0, "@alice:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 1, "@alice:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 2, "@alice:localhost"))
	userIDs, latest, err := db.KeyChanges(ctx, 0, 0, sarama.OffsetNewest)
	if err != nil {
		t.Fatalf("Failed to KeyChanges: %s", err)
	}
	if latest != 2 {
		t.Fatalf("KeyChanges: got latest=%d want 2", latest)
	}
	if !reflect.DeepEqual(userIDs, []string{"@alice:localhost"}) {
		t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
	}
}

func TestKeyChangesUpperLimit(t *testing.T) {
	db, err := NewDatabase("file::memory:", nil)
	if err != nil {
		t.Fatalf("Failed to NewDatabase: %s", err)
	}
	MustNotError(t, db.StoreKeyChange(ctx, 0, 0, "@alice:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 1, "@bob:localhost"))
	MustNotError(t, db.StoreKeyChange(ctx, 0, 2, "@charlie:localhost"))
	userIDs, latest, err := db.KeyChanges(ctx, 0, 0, 1)
	if err != nil {
		t.Fatalf("Failed to KeyChanges: %s", err)
	}
	if latest != 1 {
		t.Fatalf("KeyChanges: got latest=%d want 1", latest)
	}
	if !reflect.DeepEqual(userIDs, []string{"@bob:localhost"}) {
		t.Fatalf("KeyChanges: wrong user_ids: %v", userIDs)
	}
}

// The purpose of this test is to make sure that the storage layer is generating sequential stream IDs per user,
// and that they are returned correctly when querying for device keys.
func TestDeviceKeysStreamIDGeneration(t *testing.T) {
	db, err := NewDatabase("file::memory:", nil)
	if err != nil {
		t.Fatalf("Failed to NewDatabase: %s", err)
	}
	alice := "@alice:TestDeviceKeysStreamIDGeneration"
	bob := "@bob:TestDeviceKeysStreamIDGeneration"
	msgs := []api.DeviceMessage{
		{
			DeviceKeys: api.DeviceKeys{
				DeviceID: "AAA",
				UserID:   alice,
				KeyJSON:  []byte(`{"key":"v1"}`),
			},
			// StreamID: 1
		},
		{
			DeviceKeys: api.DeviceKeys{
				DeviceID: "AAA",
				UserID:   bob,
				KeyJSON:  []byte(`{"key":"v1"}`),
			},
			// StreamID: 1 as this is a different user
		},
		{
			DeviceKeys: api.DeviceKeys{
				DeviceID: "another_device",
				UserID:   alice,
				KeyJSON:  []byte(`{"key":"v1"}`),
			},
			// StreamID: 2 as this is a 2nd device key
		},
	}
	MustNotError(t, db.StoreDeviceKeys(ctx, msgs))
	if msgs[0].StreamID != 1 {
		t.Fatalf("Expected StoreDeviceKeys to set StreamID=1 but got %d", msgs[0].StreamID)
	}
	if msgs[1].StreamID != 1 {
		t.Fatalf("Expected StoreDeviceKeys to set StreamID=1 (different user) but got %d", msgs[1].StreamID)
	}
	if msgs[2].StreamID != 2 {
		t.Fatalf("Expected StoreDeviceKeys to set StreamID=2 (another device) but got %d", msgs[2].StreamID)
	}

	// updating a device sets the next stream ID for that user
	msgs = []api.DeviceMessage{
		{
			DeviceKeys: api.DeviceKeys{
				DeviceID: "AAA",
				UserID:   alice,
				KeyJSON:  []byte(`{"key":"v2"}`),
			},
			// StreamID: 3
		},
	}
	MustNotError(t, db.StoreDeviceKeys(ctx, msgs))
	if msgs[0].StreamID != 3 {
		t.Fatalf("Expected StoreDeviceKeys to set StreamID=3 (new key same device) but got %d", msgs[0].StreamID)
	}

	// Querying for device keys returns the latest stream IDs
	msgs, err = db.DeviceKeysForUser(ctx, alice, []string{"AAA", "another_device"})
	if err != nil {
		t.Fatalf("DeviceKeysForUser returned error: %s", err)
	}
	wantStreamIDs := map[string]int{
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
}
