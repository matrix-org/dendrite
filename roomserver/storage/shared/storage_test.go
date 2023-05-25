package shared_test

import (
	"context"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/stretchr/testify/assert"

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

	db, err := sqlutil.Open(dbOpts, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)

	var membershipTable tables.Membership
	var stateKeyTable tables.EventStateKeys
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		err = postgres.CreateMembershipTable(db)
		assert.NoError(t, err)
		membershipTable, err = postgres.PrepareMembershipTable(db)
		assert.NoError(t, err)
		stateKeyTable, err = postgres.PrepareEventStateKeysTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateEventStateKeysTable(db)
		assert.NoError(t, err)
		err = sqlite3.CreateMembershipTable(db)
		assert.NoError(t, err)
		membershipTable, err = sqlite3.PrepareMembershipTable(db)
		assert.NoError(t, err)
		stateKeyTable, err = sqlite3.PrepareEventStateKeysTable(db)
	}
	assert.NoError(t, err)

	cache := caching.NewRistrettoCache(8*1024*1024, time.Hour, false)

	evDb := shared.EventDatabase{EventStateKeysTable: stateKeyTable, Cache: cache}

	return &shared.Database{
			DB:              db,
			EventDatabase:   evDb,
			MembershipTable: membershipTable,
			Writer:          sqlutil.NewExclusiveWriter(),
			Cache:           cache,
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
