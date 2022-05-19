package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/storage"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/dendrite/test/testrig"
	"github.com/stretchr/testify/assert"
)

func mustCreateFederationDatabase(t *testing.T, dbType test.DBType) (storage.Database, func()) {
	b, baseClose := testrig.CreateBaseDendrite(t, dbType)
	connStr, dbClose := test.PrepareDBConnectionString(t, dbType)
	db, err := storage.NewDatabase(b, &config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, b.Caches, b.Cfg.Global.ServerName)
	if err != nil {
		t.Fatalf("NewDatabase returned %s", err)
	}
	return db, func() {
		dbClose()
		baseClose()
	}
}

func TestExpireEDUs(t *testing.T) {
	var expireEDUTypes = map[string]time.Duration{
		"m.receipt": time.Millisecond,
	}

	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateFederationDatabase(t, dbType)
		defer close()
		// insert some data
		for i := 0; i < 100; i++ {
			receipt, err := db.StoreJSON(ctx, "{}")
			assert.NoError(t, err)

			err = db.AssociateEDUWithDestination(ctx, "localhost", receipt, "m.receipt", expireEDUTypes)
			assert.NoError(t, err)
		}
		// add data without expiry
		receipt, err := db.StoreJSON(ctx, "{}")
		assert.NoError(t, err)

		err = db.AssociateEDUWithDestination(ctx, "localhost", receipt, "m.read_marker", expireEDUTypes)
		assert.NoError(t, err)

		// Delete expired EDUs
		err = db.DeleteExpiredEDUs(ctx)
		assert.NoError(t, err)

		// verify the data is gone
		data, err := db.GetPendingEDUs(ctx, "localhost", 100)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(data))
	})
}
