package tables_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/matrix-org/dendrite/federationapi/storage/postgres"
	"github.com/matrix-org/dendrite/federationapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/federationapi/storage/tables"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/stretchr/testify/assert"
)

type AssumedOfflineDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.FederationAssumedOffline
}

func mustCreateAssumedOfflineTable(t *testing.T, dbType test.DBType) (database AssumedOfflineDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.FederationAssumedOffline
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresAssumedOfflineTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteAssumedOfflineTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = AssumedOfflineDatabase{
		DB:     db,
		Writer: sqlutil.NewDummyWriter(),
		Table:  tab,
	}
	return database, close
}

func TestShouldInsertAssumedOfflineServer(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateAssumedOfflineTable(t, dbType)
		defer close()

		err := db.Table.InsertAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed inserting server: %s", err.Error())
		}

		isOffline, err := db.Table.SelectAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving server: %s", err.Error())
		}

		assert.Equal(t, isOffline, true)
	})
}

func TestShouldDeleteCorrectAssumedOfflineServer(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateAssumedOfflineTable(t, dbType)
		defer close()

		err := db.Table.InsertAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed inserting server: %s", err.Error())
		}
		err = db.Table.InsertAssumedOffline(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed inserting server: %s", err.Error())
		}

		isOffline, err := db.Table.SelectAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline, true)

		err = db.Table.DeleteAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed deleting server: %s", err.Error())
		}

		isOffline, err = db.Table.SelectAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline, false)

		isOffline2, err := db.Table.SelectAssumedOffline(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline2, true)
	})
}

func TestShouldDeleteAllAssumedOfflineServers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateAssumedOfflineTable(t, dbType)
		defer close()

		err := db.Table.InsertAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed inserting server: %s", err.Error())
		}
		err = db.Table.InsertAssumedOffline(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed inserting server: %s", err.Error())
		}

		isOffline, err := db.Table.SelectAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline, true)
		isOffline2, err := db.Table.SelectAssumedOffline(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}

		assert.Equal(t, isOffline2, true)

		err = db.Table.DeleteAllAssumedOffline(ctx, nil)
		if err != nil {
			t.Fatalf("Failed deleting server: %s", err.Error())
		}

		isOffline, err = db.Table.SelectAssumedOffline(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline, false)
		isOffline2, err = db.Table.SelectAssumedOffline(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving server status: %s", err.Error())
		}
		assert.Equal(t, isOffline2, false)
	})
}
