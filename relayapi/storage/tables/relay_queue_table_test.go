// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tables_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/storage/postgres"
	"github.com/matrix-org/dendrite/relayapi/storage/sqlite3"
	"github.com/matrix-org/dendrite/relayapi/storage/tables"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

type RelayQueueDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.RelayQueue
}

func mustCreateQueueTable(
	t *testing.T,
	dbType test.DBType,
) (database RelayQueueDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.RelayQueue
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresRelayQueueTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteRelayQueueTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = RelayQueueDatabase{
		DB:     db,
		Writer: sqlutil.NewDummyWriter(),
		Table:  tab,
	}
	return database, close
}

func TestShoudInsertQueueTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueTable(t, dbType)
		defer close()

		transactionID := gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
		serverName := gomatrixserverlib.ServerName("domain")
		nid := int64(1)
		err := db.Table.InsertQueueEntry(ctx, nil, transactionID, serverName, nid)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
	})
}

func TestShouldRetrieveInsertedQueueTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueTable(t, dbType)
		defer close()

		transactionID := gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
		serverName := gomatrixserverlib.ServerName("domain")
		nid := int64(1)

		err := db.Table.InsertQueueEntry(ctx, nil, transactionID, serverName, nid)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		retrievedNids, err := db.Table.SelectQueueEntries(ctx, nil, serverName, 10)
		if err != nil {
			t.Fatalf("Failed retrieving transaction: %s", err.Error())
		}

		assert.Equal(t, nid, retrievedNids[0])
		assert.Equal(t, 1, len(retrievedNids))
	})
}

func TestShouldDeleteQueueTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueTable(t, dbType)
		defer close()

		transactionID := gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
		serverName := gomatrixserverlib.ServerName("domain")
		nid := int64(1)

		err := db.Table.InsertQueueEntry(ctx, nil, transactionID, serverName, nid)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			err = db.Table.DeleteQueueEntries(ctx, txn, serverName, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed deleting transaction: %s", err.Error())
		}

		count, err := db.Table.SelectQueueEntryCount(ctx, nil, serverName)
		if err != nil {
			t.Fatalf("Failed retrieving transaction count: %s", err.Error())
		}
		assert.Equal(t, int64(0), count)
	})
}

func TestShouldDeleteOnlySpecifiedQueueTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueTable(t, dbType)
		defer close()

		transactionID := gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
		serverName := gomatrixserverlib.ServerName("domain")
		nid := int64(1)
		transactionID2 := gomatrixserverlib.TransactionID(fmt.Sprintf("%d2", time.Now().UnixNano()))
		serverName2 := gomatrixserverlib.ServerName("domain2")
		nid2 := int64(2)
		transactionID3 := gomatrixserverlib.TransactionID(fmt.Sprintf("%d3", time.Now().UnixNano()))

		err := db.Table.InsertQueueEntry(ctx, nil, transactionID, serverName, nid)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertQueueEntry(ctx, nil, transactionID2, serverName2, nid)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
		err = db.Table.InsertQueueEntry(ctx, nil, transactionID3, serverName, nid2)
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			err = db.Table.DeleteQueueEntries(ctx, txn, serverName, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed deleting transaction: %s", err.Error())
		}

		count, err := db.Table.SelectQueueEntryCount(ctx, nil, serverName)
		if err != nil {
			t.Fatalf("Failed retrieving transaction count: %s", err.Error())
		}
		assert.Equal(t, int64(1), count)

		count, err = db.Table.SelectQueueEntryCount(ctx, nil, serverName2)
		if err != nil {
			t.Fatalf("Failed retrieving transaction count: %s", err.Error())
		}
		assert.Equal(t, int64(1), count)
	})
}
