package tables_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/element-hq/dendrite/federationapi/storage/postgres"
	"github.com/element-hq/dendrite/federationapi/storage/sqlite3"
	"github.com/element-hq/dendrite/federationapi/storage/tables"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
)

func mustCreateInboundpeeksTable(t *testing.T, dbType test.DBType) (tables.FederationInboundPeeks, func()) {
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open database: %s", err)
	}
	var tab tables.FederationInboundPeeks
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresInboundPeeksTable(db)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteInboundPeeksTable(db)
	}
	if err != nil {
		t.Fatalf("failed to create table: %s", err)
	}
	return tab, close
}

func TestInboundPeeksTable(t *testing.T) {
	ctx := context.Background()
	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)
	_, serverName, _ := gomatrixserverlib.SplitID('@', alice.ID)
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, closeDB := mustCreateInboundpeeksTable(t, dbType)
		defer closeDB()

		// Insert a peek
		peekID := util.RandomString(8)
		var renewalInterval int64 = 1000
		if err := tab.InsertInboundPeek(ctx, nil, serverName, room.ID, peekID, renewalInterval); err != nil {
			t.Fatal(err)
		}

		// select the newly inserted peek
		inboundPeek1, err := tab.SelectInboundPeek(ctx, nil, serverName, room.ID, peekID)
		if err != nil {
			t.Fatal(err)
		}

		// Assert fields are set as expected
		if inboundPeek1.PeekID != peekID {
			t.Fatalf("unexpected inbound peek ID: %s, want %s", inboundPeek1.PeekID, peekID)
		}
		if inboundPeek1.RoomID != room.ID {
			t.Fatalf("unexpected inbound peek room ID: %s, want %s", inboundPeek1.RoomID, peekID)
		}
		if inboundPeek1.ServerName != serverName {
			t.Fatalf("unexpected inbound peek servername: %s, want %s", inboundPeek1.ServerName, serverName)
		}
		if inboundPeek1.RenewalInterval != renewalInterval {
			t.Fatalf("unexpected inbound peek renewal interval: %d, want %d", inboundPeek1.RenewalInterval, renewalInterval)
		}

		// Renew the peek
		if err = tab.RenewInboundPeek(ctx, nil, serverName, room.ID, peekID, 2000); err != nil {
			t.Fatal(err)
		}

		// verify the values changed
		inboundPeek2, err := tab.SelectInboundPeek(ctx, nil, serverName, room.ID, peekID)
		if err != nil {
			t.Fatal(err)
		}
		if reflect.DeepEqual(inboundPeek1, inboundPeek2) {
			t.Fatal("expected a change peek, but they are the same")
		}
		if inboundPeek1.ServerName != inboundPeek2.ServerName {
			t.Fatalf("unexpected servername change: %s -> %s", inboundPeek1.ServerName, inboundPeek2.ServerName)
		}
		if inboundPeek1.RoomID != inboundPeek2.RoomID {
			t.Fatalf("unexpected roomID change: %s -> %s", inboundPeek1.RoomID, inboundPeek2.RoomID)
		}

		// delete the peek
		if err = tab.DeleteInboundPeek(ctx, nil, serverName, room.ID, peekID); err != nil {
			t.Fatal(err)
		}

		// There should be no peek anymore
		peek, err := tab.SelectInboundPeek(ctx, nil, serverName, room.ID, peekID)
		if err != nil {
			t.Fatal(err)
		}
		if peek != nil {
			t.Fatalf("got a peek which should be deleted: %+v", peek)
		}

		// insert some peeks
		var peekIDs []string
		for i := 0; i < 5; i++ {
			peekID = util.RandomString(8)
			if err = tab.InsertInboundPeek(ctx, nil, serverName, room.ID, peekID, 1000); err != nil {
				t.Fatal(err)
			}
			peekIDs = append(peekIDs, peekID)
		}

		// Now select them
		inboundPeeks, err := tab.SelectInboundPeeks(ctx, nil, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(inboundPeeks) != len(peekIDs) {
			t.Fatalf("inserted %d peeks, selected %d", len(peekIDs), len(inboundPeeks))
		}
		gotPeekIDs := make([]string, 0, len(inboundPeeks))
		for _, p := range inboundPeeks {
			gotPeekIDs = append(gotPeekIDs, p.PeekID)
		}
		assert.ElementsMatch(t, gotPeekIDs, peekIDs)

		// And delete them again
		if err = tab.DeleteInboundPeeks(ctx, nil, room.ID); err != nil {
			t.Fatal(err)
		}

		// they should be gone now
		inboundPeeks, err = tab.SelectInboundPeeks(ctx, nil, room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(inboundPeeks) > 0 {
			t.Fatal("got inbound peeks which should be deleted")
		}

	})
}
