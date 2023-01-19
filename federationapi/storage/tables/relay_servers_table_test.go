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
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

const (
	server1 = "server1"
	server2 = "server2"
	server3 = "server3"
	server4 = "server4"
)

type RelayServersDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.FederationRelayServers
}

func mustCreateRelayServersTable(
	t *testing.T,
	dbType test.DBType,
) (database RelayServersDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.FederationRelayServers
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresRelayServersTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteRelayServersTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = RelayServersDatabase{
		DB:     db,
		Writer: sqlutil.NewDummyWriter(),
		Table:  tab,
	}
	return database, close
}

func Equal(a, b []gomatrixserverlib.ServerName) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func TestShouldInsertRelayServers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRelayServersTable(t, dbType)
		defer close()
		expectedRelayServers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertRelayServers(ctx, nil, server1, expectedRelayServers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		relayServers, err := db.Table.SelectRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}

		if !Equal(relayServers, expectedRelayServers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers, relayServers)
		}
	})
}

func TestShouldInsertRelayServersWithDuplicates(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRelayServersTable(t, dbType)
		defer close()
		insertRelayServers := []gomatrixserverlib.ServerName{server2, server2, server2, server3, server2}
		expectedRelayServers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertRelayServers(ctx, nil, server1, insertRelayServers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		// Insert the same list again, this shouldn't fail and should have no effect.
		err = db.Table.InsertRelayServers(ctx, nil, server1, insertRelayServers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		relayServers, err := db.Table.SelectRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}

		if !Equal(relayServers, expectedRelayServers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers, relayServers)
		}
	})
}

func TestShouldGetRelayServersUnknownDestination(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRelayServersTable(t, dbType)
		defer close()

		// Query relay servers for a destination that doesn't exist in the table.
		relayServers, err := db.Table.SelectRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}

		if !Equal(relayServers, []gomatrixserverlib.ServerName{}) {
			t.Fatalf("Expected: %v \nActual: %v", []gomatrixserverlib.ServerName{}, relayServers)
		}
	})
}

func TestShouldDeleteCorrectRelayServers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRelayServersTable(t, dbType)
		defer close()
		relayServers1 := []gomatrixserverlib.ServerName{server2, server3}
		relayServers2 := []gomatrixserverlib.ServerName{server1, server3, server4}

		err := db.Table.InsertRelayServers(ctx, nil, server1, relayServers1)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertRelayServers(ctx, nil, server2, relayServers2)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		err = db.Table.DeleteRelayServers(ctx, nil, server1, []gomatrixserverlib.ServerName{server2})
		if err != nil {
			t.Fatalf("Failed deleting relay servers for %s: %s", server1, err.Error())
		}
		err = db.Table.DeleteRelayServers(ctx, nil, server2, []gomatrixserverlib.ServerName{server1, server4})
		if err != nil {
			t.Fatalf("Failed deleting relay servers for %s: %s", server2, err.Error())
		}

		expectedRelayServers := []gomatrixserverlib.ServerName{server3}
		relayServers, err := db.Table.SelectRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}
		if !Equal(relayServers, expectedRelayServers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers, relayServers)
		}
		relayServers, err = db.Table.SelectRelayServers(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}
		if !Equal(relayServers, expectedRelayServers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers, relayServers)
		}
	})
}

func TestShouldDeleteAllRelayServers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateRelayServersTable(t, dbType)
		defer close()
		expectedRelayServers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertRelayServers(ctx, nil, server1, expectedRelayServers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertRelayServers(ctx, nil, server2, expectedRelayServers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		err = db.Table.DeleteAllRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed deleting relay servers for %s: %s", server1, err.Error())
		}

		expectedRelayServers1 := []gomatrixserverlib.ServerName{}
		relayServers, err := db.Table.SelectRelayServers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}
		if !Equal(relayServers, expectedRelayServers1) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers1, relayServers)
		}
		relayServers, err = db.Table.SelectRelayServers(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving relay servers for %s: %s", relayServers, err.Error())
		}
		if !Equal(relayServers, expectedRelayServers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedRelayServers, relayServers)
		}
	})
}
