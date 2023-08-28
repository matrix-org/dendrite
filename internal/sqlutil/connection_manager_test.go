package sqlutil_test

import (
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/setup/process"
	"github.com/matrix-org/dendrite/test"
)

func TestConnectionManager(t *testing.T) {

	t.Run("component defined connection string", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			conStr, close := test.PrepareDBConnectionString(t, dbType)
			t.Cleanup(close)
			cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})

			dbProps := &config.DatabaseOptions{ConnectionString: config.DataSource(conStr)}
			db, writer, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}

			switch dbType {
			case test.DBTypeSQLite:
				_, ok := writer.(*sqlutil.ExclusiveWriter)
				if !ok {
					t.Fatalf("expected exclusive writer")
				}
			case test.DBTypePostgres:
				_, ok := writer.(*sqlutil.DummyWriter)
				if !ok {
					t.Fatalf("expected dummy writer")
				}
			}

			// reuse existing connection
			db2, writer2, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(db, db2) {
				t.Fatalf("expected database connection to be reused")
			}
			if !reflect.DeepEqual(writer, writer2) {
				t.Fatalf("expected database writer to be reused")
			}

			// This test does not work with Postgres, because we can't just simply append
			// "x" or replace the database to use.
			if dbType == test.DBTypePostgres {
				return
			}

			// Test different connection string
			dbProps = &config.DatabaseOptions{ConnectionString: config.DataSource(conStr + "x")}
			db3, _, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}
			if reflect.DeepEqual(db, db3) {
				t.Fatalf("expected different database connection")
			}
		})
	})

	t.Run("global connection pool", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			conStr, close := test.PrepareDBConnectionString(t, dbType)
			t.Cleanup(close)
			cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{ConnectionString: config.DataSource(conStr)})

			dbProps := &config.DatabaseOptions{}
			db, writer, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}

			switch dbType {
			case test.DBTypeSQLite:
				_, ok := writer.(*sqlutil.ExclusiveWriter)
				if !ok {
					t.Fatalf("expected exclusive writer")
				}
			case test.DBTypePostgres:
				_, ok := writer.(*sqlutil.DummyWriter)
				if !ok {
					t.Fatalf("expected dummy writer")
				}
			}

			// reuse existing connection
			db2, writer2, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(db, db2) {
				t.Fatalf("expected database connection to be reused")
			}
			if !reflect.DeepEqual(writer, writer2) {
				t.Fatalf("expected database writer to be reused")
			}
		})
	})

	t.Run("shutdown", func(t *testing.T) {
		test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
			conStr, close := test.PrepareDBConnectionString(t, dbType)
			t.Cleanup(close)

			processCtx := process.NewProcessContext()
			cm := sqlutil.NewConnectionManager(processCtx, config.DatabaseOptions{ConnectionString: config.DataSource(conStr)})

			dbProps := &config.DatabaseOptions{}
			_, _, err := cm.Connection(dbProps)
			if err != nil {
				t.Fatal(err)
			}

			processCtx.ShutdownDendrite()
			processCtx.WaitForComponentsToFinish()
		})
	})

	// test invalid connection string configured
	cm2 := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
	_, _, err := cm2.Connection(&config.DatabaseOptions{ConnectionString: "http://"})
	if err == nil {
		t.Fatal("expected an error but got none")
	}

	// empty connection string is not allowed
	_, _, err = cm2.Connection(&config.DatabaseOptions{})
	if err == nil {
		t.Fatal("expected an error but got none")
	}
}
