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
)

type MailserversDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.FederationMailservers
}

func mustCreateMailserversTable(t *testing.T, dbType test.DBType) (database MailserversDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.FederationMailservers
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresMailserversTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteMailserversTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = MailserversDatabase{
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

func TestShouldInsertMailservers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateMailserversTable(t, dbType)
		defer close()
		expectedMailservers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertMailservers(ctx, nil, server1, expectedMailservers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		mailservers, err := db.Table.SelectMailservers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving mailservers for %s: %s", mailservers, err.Error())
		}

		if !Equal(mailservers, expectedMailservers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedMailservers, mailservers)
		}
	})
}

func TestShouldDeleteCorrectMailservers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateMailserversTable(t, dbType)
		defer close()
		expectedMailservers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertMailservers(ctx, nil, server1, expectedMailservers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertMailservers(ctx, nil, server2, expectedMailservers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		err = db.Table.DeleteMailservers(ctx, nil, server1, []gomatrixserverlib.ServerName{server2})
		if err != nil {
			t.Fatalf("Failed deleting mailservers for %s: %s", server1, err.Error())
		}

		expectedMailservers1 := []gomatrixserverlib.ServerName{server3}
		mailservers, err := db.Table.SelectMailservers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving mailservers for %s: %s", mailservers, err.Error())
		}
		if !Equal(mailservers, expectedMailservers1) {
			t.Fatalf("Expected: %v \nActual: %v", expectedMailservers1, mailservers)
		}
		mailservers, err = db.Table.SelectMailservers(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving mailservers for %s: %s", mailservers, err.Error())
		}
		if !Equal(mailservers, expectedMailservers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedMailservers, mailservers)
		}
	})
}

func TestShouldDeleteAllMailservers(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateMailserversTable(t, dbType)
		defer close()
		expectedMailservers := []gomatrixserverlib.ServerName{server2, server3}

		err := db.Table.InsertMailservers(ctx, nil, server1, expectedMailservers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertMailservers(ctx, nil, server2, expectedMailservers)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		err = db.Table.DeleteAllMailservers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed deleting mailservers for %s: %s", server1, err.Error())
		}

		expectedMailservers1 := []gomatrixserverlib.ServerName{}
		mailservers, err := db.Table.SelectMailservers(ctx, nil, server1)
		if err != nil {
			t.Fatalf("Failed retrieving mailservers for %s: %s", mailservers, err.Error())
		}
		if !Equal(mailservers, expectedMailservers1) {
			t.Fatalf("Expected: %v \nActual: %v", expectedMailservers1, mailservers)
		}
		mailservers, err = db.Table.SelectMailservers(ctx, nil, server2)
		if err != nil {
			t.Fatalf("Failed retrieving mailservers for %s: %s", mailservers, err.Error())
		}
		if !Equal(mailservers, expectedMailservers) {
			t.Fatalf("Expected: %v \nActual: %v", expectedMailservers, mailservers)
		}
	})
}
