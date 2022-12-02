package routing_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	testOrigin      = gomatrixserverlib.ServerName("kaer.morhen")
	testDestination = gomatrixserverlib.ServerName("white.orchard")
)

type testDatabase struct {
	nid          int64
	nidMutex     sync.Mutex
	transactions map[int64]json.RawMessage
	associations map[gomatrixserverlib.ServerName][]int64
}

func createDatabase() *testDatabase {
	return &testDatabase{
		nid:          1,
		nidMutex:     sync.Mutex{},
		transactions: make(map[int64]json.RawMessage),
		associations: make(map[gomatrixserverlib.ServerName][]int64),
	}
}

func (d *testDatabase) InsertQueueTransaction(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error {
	if _, ok := d.associations[serverName]; !ok {
		d.associations[serverName] = []int64{}
	}
	d.associations[serverName] = append(d.associations[serverName], nid)
	return nil
}

func (d *testDatabase) DeleteQueueTransactions(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error {
	for _, nid := range jsonNIDs {
		for index, associatedNID := range d.associations[serverName] {
			if associatedNID == nid {
				d.associations[serverName] = append(d.associations[serverName][:index], d.associations[serverName][index+1:]...)
			}
		}
	}

	return nil
}

func (d *testDatabase) SelectQueueTransactions(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error) {
	results := []int64{}
	resultCount := limit
	if limit > len(d.associations[serverName]) {
		resultCount = len(d.associations[serverName])
	}
	if resultCount > 0 {
		for i := 0; i < resultCount; i++ {
			results = append(results, d.associations[serverName][i])
		}
	}

	return results, nil
}

func (d *testDatabase) SelectQueueTransactionCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error) {
	return int64(len(d.associations[serverName])), nil
}

func (d *testDatabase) InsertTransactionJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error) {
	d.nidMutex.Lock()
	defer d.nidMutex.Unlock()

	nid := d.nid
	d.transactions[nid] = []byte(json)
	d.nid++

	return nid, nil
}

func (d *testDatabase) DeleteTransactionJSON(ctx context.Context, txn *sql.Tx, nids []int64) error {
	for _, nid := range nids {
		delete(d.transactions, nid)
	}

	return nil
}

func (d *testDatabase) SelectTransactionJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error) {
	result := make(map[int64][]byte)
	for _, nid := range jsonNIDs {
		if transaction, ok := d.transactions[nid]; ok {
			result[nid] = transaction
		}
	}

	return result, nil
}

func createTransaction() gomatrixserverlib.Transaction {
	txn := gomatrixserverlib.Transaction{}
	txn.PDUs = []json.RawMessage{
		[]byte(`{"auth_events":[["$0ok8ynDp7kjc95e3:kaer.morhen",{"sha256":"sWCi6Ckp9rDimQON+MrUlNRkyfZ2tjbPbWfg2NMB18Q"}],["$LEwEu0kxrtu5fOiS:kaer.morhen",{"sha256":"1aKajq6DWHru1R1HJjvdWMEavkJJHGaTmPvfuERUXaA"}]],"content":{"body":"Test Message"},"depth":5,"event_id":"$gl2T9l3qm0kUbiIJ:kaer.morhen","hashes":{"sha256":"Qx3nRMHLDPSL5hBAzuX84FiSSP0K0Kju2iFoBWH4Za8"},"origin":"kaer.morhen","origin_server_ts":0,"prev_events":[["$UKNe10XzYzG0TeA9:kaer.morhen",{"sha256":"KtSRyMjt0ZSjsv2koixTRCxIRCGoOp6QrKscsW97XRo"}]],"room_id":"!roomid:kaer.morhen","sender":"@userid:kaer.morhen","signatures":{"kaer.morhen":{"ed25519:auto":"sqDgv3EG7ml5VREzmT9aZeBpS4gAPNIaIeJOwqjDhY0GPU/BcpX5wY4R7hYLrNe5cChgV+eFy/GWm1Zfg5FfDg"}},"type":"m.room.message"}`),
	}
	txn.Origin = testOrigin
	txn.TransactionID = gomatrixserverlib.TransactionID(fmt.Sprintf("%d", time.Now().UnixNano()))
	txn.Destination = testDestination
	return txn
}

func createFederationRequest(userID gomatrixserverlib.UserID) (gomatrixserverlib.Transaction, gomatrixserverlib.FederationRequest) {
	txn := createTransaction()
	var federationPathPrefixV1 = "/_matrix/federation/v1"
	path := federationPathPrefixV1 + "/forward_async/" + string(txn.TransactionID) + "/" + userID.Raw()
	request := gomatrixserverlib.NewFederationRequest("PUT", txn.Origin, txn.Destination, path)
	request.SetContent(txn)

	return txn, request
}

func TestEmptyForwardReturnsOk(t *testing.T) {
	testDB := createDatabase()
	db := shared.Database{
		Writer:                      sqlutil.NewDummyWriter(),
		FederationQueueTransactions: testDB,
		FederationTransactionJSON:   testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	if err != nil {
		t.Fatalf("Invalid userID: %s", err.Error())
	}
	_, request := createFederationRequest(*userID)

	fedAPI := internal.NewFederationInternalAPI(
		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
	)

	response := routing.ForwardAsync(httpReq, &request, fedAPI, "1", *userID)

	expected := 200
	if response.Code != expected {
		t.Fatalf("Expected: %v, Actual: %v", expected, response.Code)
	}
}

func TestUniqueTransactionStoredInDatabase(t *testing.T) {
	testDB := createDatabase()
	db := shared.Database{
		Writer:                      sqlutil.NewDummyWriter(),
		FederationQueueTransactions: testDB,
		FederationTransactionJSON:   testDB,
	}

	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	if err != nil {
		t.Fatalf("Invalid userID: %s", err.Error())
	}
	inputTransaction, request := createFederationRequest(*userID)

	fedAPI := internal.NewFederationInternalAPI(
		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
	)

	response := routing.ForwardAsync(
		httpReq, &request, fedAPI, inputTransaction.TransactionID, *userID)
	transaction, _, err := db.GetAsyncTransaction(context.TODO(), *userID)
	if err != nil {
		t.Fatalf("Failed retrieving transaction: %s", err.Error())
	}
	transactionCount, err := db.GetAsyncTransactionCount(context.TODO(), *userID)
	if err != nil {
		t.Fatalf("Failed retrieving transaction count: %s", err.Error())
	}

	expected := 200
	if response.Code != expected {
		t.Fatalf("Expected Return Code: %v, Actual: %v", expected, response.Code)
	}
	if transactionCount != 1 {
		t.Fatalf("Expected count of 1, Actual: %d", transactionCount)
	}
	if transaction.TransactionID != inputTransaction.TransactionID {
		t.Fatalf("Expected Transaction ID: %s, Actual: %s",
			inputTransaction.TransactionID, transaction.TransactionID)
	}
}
