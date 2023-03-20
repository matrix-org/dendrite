package sqlutil_test

import (
	"reflect"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
)

func TestConnectionManager(t *testing.T) {
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		conStr, close := test.PrepareDBConnectionString(t, dbType)
		t.Cleanup(close)
		cm := sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})

		dbProps := &config.DatabaseOptions{ConnectionString: config.DataSource(string(conStr))}
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

		// test global db pool
		dbGlobal, writerGlobal, err := cm.Connection(&config.DatabaseOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(db, dbGlobal) {
			t.Fatalf("expected database connection to be reused")
		}
		if !reflect.DeepEqual(writer, writerGlobal) {
			t.Fatalf("expected database writer to be reused")
		}

		// test invalid connection string configured
		cm = sqlutil.NewConnectionManager(nil, config.DatabaseOptions{})
		_, _, err = cm.Connection(&config.DatabaseOptions{ConnectionString: "http://"})
		if err == nil {
			t.Fatal("expected an error but got none")
		}
	})
}
