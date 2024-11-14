package tables_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/matrix-org/gomatrixserverlib/spec"

	"github.com/element-hq/dendrite/internal/sqlutil"
	rstypes "github.com/element-hq/dendrite/roomserver/types"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/syncapi/storage/postgres"
	"github.com/element-hq/dendrite/syncapi/storage/sqlite3"
	"github.com/element-hq/dendrite/syncapi/storage/tables"
	"github.com/element-hq/dendrite/syncapi/types"
	"github.com/element-hq/dendrite/test"
)

func newMembershipsTable(t *testing.T, dbType test.DBType) (tables.Memberships, *sql.DB, func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	if err != nil {
		t.Fatalf("failed to open db: %s", err)
	}

	var tab tables.Memberships
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresMembershipsTable(db)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSqliteMembershipsTable(db)
	}
	if err != nil {
		t.Fatalf("failed to make new table: %s", err)
	}
	return tab, db, close
}

func TestMembershipsTable(t *testing.T) {

	alice := test.NewUser(t)
	room := test.NewRoom(t, alice)

	// Create users
	var userEvents []*rstypes.HeaderedEvent
	users := []string{alice.ID}
	for _, x := range room.CurrentState() {
		if x.StateKeyEquals(alice.ID) {
			if _, err := x.Membership(); err == nil {
				userEvents = append(userEvents, x)
				break
			}
		}
	}

	if len(userEvents) == 0 {
		t.Fatalf("didn't find creator membership event")
	}

	for i := 0; i < 10; i++ {
		u := test.NewUser(t)
		users = append(users, u.ID)

		ev := room.CreateAndInsert(t, u, spec.MRoomMember, map[string]interface{}{
			"membership": "join",
		}, test.WithStateKey(u.ID))
		userEvents = append(userEvents, ev)
	}

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		table, _, close := newMembershipsTable(t, dbType)
		defer close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		for _, ev := range userEvents {
			ev.StateKeyResolved = ev.StateKey()
			if err := table.UpsertMembership(ctx, nil, ev, types.StreamPosition(ev.Depth()), 1); err != nil {
				t.Fatalf("failed to upsert membership: %s", err)
			}
		}

		testUpsert(t, ctx, table, userEvents[0], alice, room)
		testMembershipCount(t, ctx, table, room)
	})
}

func testMembershipCount(t *testing.T, ctx context.Context, table tables.Memberships, room *test.Room) {
	t.Run("membership counts are correct", func(t *testing.T) {
		// After 10 events, we should have 6 users (5 create related [incl. one member event], 5 member events = 6 users)
		count, err := table.SelectMembershipCount(ctx, nil, room.ID, spec.Join, 10)
		if err != nil {
			t.Fatalf("failed to get membership count: %s", err)
		}
		expectedCount := 6
		if expectedCount != count {
			t.Fatalf("expected member count to be %d, got %d", expectedCount, count)
		}

		// After 100 events, we should have all 11 users
		count, err = table.SelectMembershipCount(ctx, nil, room.ID, spec.Join, 100)
		if err != nil {
			t.Fatalf("failed to get membership count: %s", err)
		}
		expectedCount = 11
		if expectedCount != count {
			t.Fatalf("expected member count to be %d, got %d", expectedCount, count)
		}
	})
}

func testUpsert(t *testing.T, ctx context.Context, table tables.Memberships, membershipEvent *rstypes.HeaderedEvent, user *test.User, room *test.Room) {
	t.Run("upserting works as expected", func(t *testing.T) {
		if err := table.UpsertMembership(ctx, nil, membershipEvent, 1, 1); err != nil {
			t.Fatalf("failed to upsert membership: %s", err)
		}
		membership, pos, err := table.SelectMembershipForUser(ctx, nil, room.ID, user.ID, 1)
		if err != nil {
			t.Fatalf("failed to select membership: %s", err)
		}
		var expectedPos int64 = 1
		if pos != expectedPos {
			t.Fatalf("expected pos to be %d, got %d", expectedPos, pos)
		}
		if membership != spec.Join {
			t.Fatalf("expected membership to be join, got %s", membership)
		}
		// Create a new event which gets upserted and should not cause issues
		ev := room.CreateAndInsert(t, user, spec.MRoomMember, map[string]interface{}{
			"membership": spec.Join,
		}, test.WithStateKey(user.ID))
		ev.StateKeyResolved = ev.StateKey()
		// Insert the same event again, but with different positions, which should get updated
		if err = table.UpsertMembership(ctx, nil, ev, 2, 2); err != nil {
			t.Fatalf("failed to upsert membership: %s", err)
		}

		// Verify the position got updated
		membership, pos, err = table.SelectMembershipForUser(ctx, nil, room.ID, user.ID, 10)
		if err != nil {
			t.Fatalf("failed to select membership: %s", err)
		}
		expectedPos = 2
		if pos != expectedPos {
			t.Fatalf("expected pos to be %d, got %d", expectedPos, pos)
		}
		if membership != spec.Join {
			t.Fatalf("expected membership to be join, got %s", membership)
		}

		// If we can't find a membership, it should default to leave
		if membership, _, err = table.SelectMembershipForUser(ctx, nil, room.ID, user.ID, 1); err != nil {
			t.Fatalf("failed to select membership: %s", err)
		}
		if membership != spec.Leave {
			t.Fatalf("expected membership to be leave, got %s", membership)
		}
	})
}
