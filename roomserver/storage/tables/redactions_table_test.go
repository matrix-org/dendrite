package tables_test

import (
	"context"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/storage/postgres"
	"github.com/matrix-org/dendrite/roomserver/storage/sqlite3"
	"github.com/matrix-org/dendrite/roomserver/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/util"
	"github.com/stretchr/testify/assert"
)

func mustCreateRedactionsTable(t *testing.T, dbType test.DBType) (tab tables.Redactions, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	switch dbType {
	case test.DBTypePostgres:
		err = postgres.CreateRedactionsTable(db)
		assert.NoError(t, err)
		tab, err = postgres.PrepareRedactionsTable(db)
	case test.DBTypeSQLite:
		err = sqlite3.CreateRedactionsTable(db)
		assert.NoError(t, err)
		tab, err = sqlite3.PrepareRedactionsTable(db)
	}
	assert.NoError(t, err)

	return tab, close
}

func TestRedactionsTable(t *testing.T) {
	ctx := context.Background()

	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		tab, close := mustCreateRedactionsTable(t, dbType)
		defer close()

		// insert and verify some redactions
		for i := 0; i < 10; i++ {
			redactionEventID, redactsEventID := util.RandomString(16), util.RandomString(16)
			wantRedactionInfo := tables.RedactionInfo{
				Validated:        false,
				RedactsEventID:   redactsEventID,
				RedactionEventID: redactionEventID,
			}
			err := tab.InsertRedaction(ctx, nil, wantRedactionInfo)
			assert.NoError(t, err)

			// verify the redactions are inserted as expected
			redactionInfo, err := tab.SelectRedactionInfoByRedactionEventID(ctx, nil, redactionEventID)
			assert.NoError(t, err)
			assert.Equal(t, &wantRedactionInfo, redactionInfo)

			redactionInfo, err = tab.SelectRedactionInfoByEventBeingRedacted(ctx, nil, redactsEventID)
			assert.NoError(t, err)
			assert.Equal(t, &wantRedactionInfo, redactionInfo)

			// redact event
			err = tab.MarkRedactionValidated(ctx, nil, redactionEventID, true)
			assert.NoError(t, err)

			wantRedactionInfo.Validated = true
			redactionInfo, err = tab.SelectRedactionInfoByRedactionEventID(ctx, nil, redactionEventID)
			assert.NoError(t, err)
			assert.Equal(t, &wantRedactionInfo, redactionInfo)
		}

		// Should not fail, it just updates 0 rows
		err := tab.MarkRedactionValidated(ctx, nil, "iDontExist", true)
		assert.NoError(t, err)

		// Should also not fail, but return a nil redactionInfo
		redactionInfo, err := tab.SelectRedactionInfoByRedactionEventID(ctx, nil, "iDontExist")
		assert.NoError(t, err)
		assert.Nil(t, redactionInfo)

		redactionInfo, err = tab.SelectRedactionInfoByEventBeingRedacted(ctx, nil, "iDontExist")
		assert.NoError(t, err)
		assert.Nil(t, redactionInfo)
	})
}
