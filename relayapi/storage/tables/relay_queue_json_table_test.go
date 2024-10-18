// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package tables_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/relayapi/storage/postgres"
	"github.com/element-hq/dendrite/relayapi/storage/sqlite3"
	"github.com/element-hq/dendrite/relayapi/storage/tables"
	"github.com/element-hq/dendrite/setup/config"
	"github.com/element-hq/dendrite/test"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/stretchr/testify/assert"
)

const (
	testOrigin = spec.ServerName("kaer.morhen")
)

func mustCreateTransaction() gomatrixserverlib.Transaction {
	txn := gomatrixserverlib.Transaction{}
	txn.PDUs = []json.RawMessage{
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":5,"event_id":"$gl2T9l3qm0kUbiIJ:kaer.morhen","hashes":{"sha256":"Qx3nRMHLDPSL5hBAzuX84FiSSP0K0Kju2iFoBWH4Za8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$UKNe10XzYzG0TeA9:kaer.morhen",{"sha256":"KtSRyMjt0ZSjsv2koixTRCxIRCGoOp6QrKscsW97XRo"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sqDgv3EG7ml5VREzmT9aZeBpS4gAPNIaIeJOwqjDhY0GPU/BcpX5wY4R7hYLrNe5cChgV+eFy/GWm1Zfg5FfDg"}},"type":"m.room.message"}`),
	}
	txn.Origin = testOrigin

	return txn
}

type RelayQueueJSONDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.RelayQueueJSON
}

func mustCreateQueueJSONTable(
	t *testing.T,
	dbType test.DBType,
) (database RelayQueueJSONDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.RelayQueueJSON
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresRelayQueueJSONTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteRelayQueueJSONTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = RelayQueueJSONDatabase{
		DB:     db,
		Writer: sqlutil.NewDummyWriter(),
		Table:  tab,
	}
	return database, close
}

func TestShoudInsertTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		_, err = db.Table.InsertQueueJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
	})
}

func TestShouldRetrieveInsertedTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		nid, err := db.Table.InsertQueueJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		var storedJSON map[int64][]byte
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			storedJSON, err = db.Table.SelectQueueJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed retrieving transaction: %s", err.Error())
		}

		assert.Equal(t, 1, len(storedJSON))

		var storedTx gomatrixserverlib.Transaction
		json.Unmarshal(storedJSON[1], &storedTx)

		assert.Equal(t, transaction, storedTx)
	})
}

func TestShouldDeleteTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateQueueJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		nid, err := db.Table.InsertQueueJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		storedJSON := map[int64][]byte{}
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			err = db.Table.DeleteQueueJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed deleting transaction: %s", err.Error())
		}

		storedJSON = map[int64][]byte{}
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			storedJSON, err = db.Table.SelectQueueJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed retrieving transaction: %s", err.Error())
		}

		assert.Equal(t, 0, len(storedJSON))
	})
}
