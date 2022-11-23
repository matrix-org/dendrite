package tables_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

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
	testOrigin      = gomatrixserverlib.ServerName("kaer.morhen")
	testDestination = gomatrixserverlib.ServerName("white.orchard")
)

func mustCreateTransaction() gomatrixserverlib.Transaction {
	txn := gomatrixserverlib.Transaction{}
	txn.PDUs = []json.RawMessage{
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":5,"event_id":"$gl2T9l3qm0kUbiIJ:kaer.morhen","hashes":{"sha256":"Qx3nRMHLDPSL5hBAzuX84FiSSP0K0Kju2iFoBWH4Za8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$UKNe10XzYzG0TeA9:kaer.morhen",{"sha256":"KtSRyMjt0ZSjsv2koixTRCxIRCGoOp6QrKscsW97XRo"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sqDgv3EG7ml5VREzmT9aZeBpS4gAPNIaIeJOwqjDhY0GPU/BcpX5wY4R7hYLrNe5cChgV+eFy/GWm1Zfg5FfDg"}},"type":"m.room.message"}`),
	}
	txn.Origin = testOrigin
	txn.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
	txn.Destination = testDestination

	return txn
}

type TransactionJSONDatabase struct {
	DB     *sql.DB
	Writer sqlutil.Writer
	Table  tables.FederationTransactionJSON
}

func mustCreateTransactionJSONTable(t *testing.T, dbType test.DBType) (database TransactionJSONDatabase, close func()) {
	t.Helper()
	connStr, close := test.PrepareDBConnectionString(t, dbType)
	db, err := sqlutil.Open(&config.DatabaseOptions{
		ConnectionString: config.DataSource(connStr),
	}, sqlutil.NewExclusiveWriter())
	assert.NoError(t, err)
	var tab tables.FederationTransactionJSON
	switch dbType {
	case test.DBTypePostgres:
		tab, err = postgres.NewPostgresTransactionJSONTable(db)
		assert.NoError(t, err)
	case test.DBTypeSQLite:
		tab, err = sqlite3.NewSQLiteTransactionJSONTable(db)
		assert.NoError(t, err)
	}
	assert.NoError(t, err)

	database = TransactionJSONDatabase{
		DB:     db,
		Writer: sqlutil.NewDummyWriter(),
		Table:  tab,
	}
	return database, close
}

func TestShoudInsertTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateTransactionJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		_, err = db.Table.InsertTransactionJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}
	})
}

func TestShouldRetrieveInsertedTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateTransactionJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		nid, err := db.Table.InsertTransactionJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		var storedJSON map[int64][]byte
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			storedJSON, err = db.Table.SelectTransactionJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed retrieving transaction: %s", err.Error())
		}

		assert.Equal(t, len(storedJSON), 1)

		var storedTx gomatrixserverlib.Transaction
		json.Unmarshal(storedJSON[1], &storedTx)

		assert.Equal(t, transaction, storedTx)
	})
}

func TestShouldDeleteTransaction(t *testing.T) {
	ctx := context.Background()
	test.WithAllDatabases(t, func(t *testing.T, dbType test.DBType) {
		db, close := mustCreateTransactionJSONTable(t, dbType)
		defer close()

		transaction := mustCreateTransaction()
		tx, err := json.Marshal(transaction)
		if err != nil {
			t.Fatalf("Invalid transaction: %s", err.Error())
		}

		nid, err := db.Table.InsertTransactionJSON(ctx, nil, string(tx))
		if err != nil {
			t.Fatalf("Failed inserting transaction: %s", err.Error())
		}

		storedJSON := map[int64][]byte{}
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			err = db.Table.DeleteTransactionJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed deleting transaction: %s", err.Error())
		}

		storedJSON = map[int64][]byte{}
		_ = db.Writer.Do(db.DB, nil, func(txn *sql.Tx) error {
			storedJSON, err = db.Table.SelectTransactionJSON(ctx, txn, []int64{nid})
			return err
		})
		if err != nil {
			t.Fatalf("Failed retrieving transaction: %s", err.Error())
		}

		assert.Equal(t, len(storedJSON), 0)
	})
}
