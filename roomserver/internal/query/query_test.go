// Copyright 2024 New Vector Ltd.
// Copyright 2020 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package query

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/roomserver/storage"
	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// used to implement RoomserverInternalAPIEventDB to test getAuthChain
type getEventDB struct {
	eventMap map[string]gomatrixserverlib.PDU
}

func createEventDB() *getEventDB {
	return &getEventDB{
		eventMap: make(map[string]gomatrixserverlib.PDU),
	}
}

// Adds a fake event to the storage with given auth events.
func (db *getEventDB) addFakeEvent(eventID string, authIDs []string) error {
	authEvents := make([]any, 0, len(authIDs))
	for _, authID := range authIDs {
		authEvents = append(authEvents, []any{authID, struct{}{}})
	}
	builder := map[string]interface{}{
		"event_id":    eventID,
		"room_id":     "!room:a",
		"auth_events": authEvents,
	}

	eventJSON, err := json.Marshal(&builder)
	if err != nil {
		return err
	}

	event, err := gomatrixserverlib.MustGetRoomVersion(gomatrixserverlib.RoomVersionV1).NewEventFromTrustedJSON(
		eventJSON, false,
	)
	if err != nil {
		return err
	}

	db.eventMap[eventID] = event

	return nil
}

// Adds multiple events at once, each entry in the map is an eventID and set of
// auth events that are converted to an event and added.
func (db *getEventDB) addFakeEvents(graph map[string][]string) error {
	for eventID, authIDs := range graph {
		err := db.addFakeEvent(eventID, authIDs)
		if err != nil {
			return err
		}
	}

	return nil
}

// EventsFromIDs implements RoomserverInternalAPIEventDB
func (db *getEventDB) EventsFromIDs(ctx context.Context, roomInfo *types.RoomInfo, eventIDs []string) (res []types.Event, err error) {
	for _, evID := range eventIDs {
		res = append(res, types.Event{
			EventNID: 0,
			PDU:      db.eventMap[evID],
		})
	}

	return
}

func TestGetAuthChainSingle(t *testing.T) {
	db := createEventDB()

	err := db.addFakeEvents(map[string][]string{
		"a": {},
		"b": {"a"},
		"c": {"a", "b"},
		"d": {"b", "c"},
		"e": {"a", "d"},
	})

	if err != nil {
		t.Fatalf("Failed to add events to db: %v", err)
	}

	result, err := GetAuthChain(context.TODO(), db.EventsFromIDs, nil, []string{"e"})
	if err != nil {
		t.Fatalf("getAuthChain failed: %v", err)
	}

	var returnedIDs []string
	for _, event := range result {
		returnedIDs = append(returnedIDs, event.EventID())
	}

	expectedIDs := []string{"a", "b", "c", "d", "e"}

	if !test.UnsortedStringSliceEqual(expectedIDs, returnedIDs) {
		t.Fatalf("returnedIDs got '%v', expected '%v'", returnedIDs, expectedIDs)
	}
}

func TestGetAuthChainMultiple(t *testing.T) {
	db := createEventDB()

	err := db.addFakeEvents(map[string][]string{
		"a": {},
		"b": {"a"},
		"c": {"a", "b"},
		"d": {"b", "c"},
		"e": {"a", "d"},
		"f": {"a", "b", "c"},
	})

	if err != nil {
		t.Fatalf("Failed to add events to db: %v", err)
	}

	result, err := GetAuthChain(context.TODO(), db.EventsFromIDs, nil, []string{"e", "f"})
	if err != nil {
		t.Fatalf("getAuthChain failed: %v", err)
	}

	var returnedIDs []string
	for _, event := range result {
		returnedIDs = append(returnedIDs, event.EventID())
	}

	expectedIDs := []string{"a", "b", "c", "d", "e", "f"}

	if !test.UnsortedStringSliceEqual(expectedIDs, returnedIDs) {
		t.Fatalf("returnedIDs got '%v', expected '%v'", returnedIDs, expectedIDs)
	}
}

func mustCreateDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	conStr, close := test.PrepareDBConnectionString(t, dbType)
	caches := caching.NewRistrettoCache(8*1024*1024, time.Hour, caching.DisableMetrics)
	cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	db, err := storage.Open(context.Background(), cm, &config.DatabaseOptions{ConnectionString: config.DataSource(conStr)}, caches)
	if err != nil {
		t.Fatalf("failed to create Database: %v", err)
	}
	return db, close
}

func TestCurrentEventIsNil(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateDatabase(t, dbType)
		defer close()
		querier := Queryer{
			DB: db,
		}

		roomID, _ := spec.NewRoomID("!room:server")
		event, _ := querier.CurrentStateEvent(context.Background(), *roomID, spec.MRoomMember, "@user:server")
		if event != nil {
			t.Fatal("Event should equal nil, most likely this is failing because the interface type is not nil, but the value is.")
		}
	})
}
