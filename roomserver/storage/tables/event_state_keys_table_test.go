package tables_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/roomserver/types"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func mustCreateEventStateKeysTable(t *testing.T, dbType test.DBType) (tables.EventStateKeys, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}
	var tab tables.EventStateKeys
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventStateKeysTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = postgres.PrepareEventStateKeysTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventStateKeysTable(db)
		if err != nil {
			t.Fatalf("failed to create EventJSON table: %s", err)
		}
		tab, err = sqlite3.PrepareEventStateKeysTable(db)
	}
	if err != nil {
		t.Fatalf("failed to create table: %s", err)
	}

	return tab, close
}

func Test_EventStateKeysTable(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateEventStateKeysTable(t, dbType)
		defer close()
		ctx := context.Background()
		var stateKeyNID, gotEventStateKey types.EventStateKeyNID
		var err error
		// create some dummy data
		for i := 0; i < 10; i++ {
			stateKey := fmt.Sprintf("@user%d:localhost", i)
			if stateKeyNID, err = tab.InsertEventStateKeyNID(
				ctx, nil, stateKey,
			); err != nil {
				t.Fatalf("unable to insert eventJSON: %s", err)
			}
			gotEventStateKey, err = tab.SelectEventStateKeyNID(ctx, nil, stateKey)
			if err != nil {
				t.Fatalf("failed to get eventStateKeyNID: %s", err)
			}
			if stateKeyNID != gotEventStateKey {
				t.Fatalf("expected eventStateKey %d, but got %d", stateKeyNID, gotEventStateKey)
			}
		}
		stateKeyNIDsMap, err := tab.BulkSelectEventStateKeyNID(ctx, nil, []string{"@user0:localhost", "@user1:localhost"})
		if err != nil {
			t.Fatalf("failed to get EventStateKeyNIDs: %s", err)
		}
		wantStateKeyNIDs := make([]types.EventStateKeyNID, 0, len(stateKeyNIDsMap))
		for _, nid := range stateKeyNIDsMap {
			wantStateKeyNIDs = append(wantStateKeyNIDs, nid)
		}
		stateKeyNIDs, err := tab.BulkSelectEventStateKey(ctx, nil, wantStateKeyNIDs)
		if err != nil {
			t.Fatalf("failed to get EventStateKeyNIDs: %s", err)
		}
		// verify that BulkSelectEventStateKeyNID and BulkSelectEventStateKey return the same values
		for userID, nid := range stateKeyNIDsMap {
			if v, ok := stateKeyNIDs[nid]; ok {
				if v != userID {
					t.Fatalf("userID does not match: %s != %s", userID, v)
				}
			} else {
				t.Fatalf("unable to find %d in result set", nid)
			}
		}
	})
}
