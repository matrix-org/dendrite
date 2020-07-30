package storage

import (
	"context"
	"reflect"
	"testing"
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
	userIDs, latest, err := db.KeyChanges(ctx, 0, 1)
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
	userIDs, latest, err := db.KeyChanges(ctx, 0, 0)
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
