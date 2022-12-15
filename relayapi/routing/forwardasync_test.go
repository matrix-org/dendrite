package routing_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/internal"
	"github.com/matrix-org/dendrite/relayapi/routing"
	"github.com/matrix-org/dendrite/relayapi/storage"
	"github.com/matrix-org/dendrite/relayapi/storage/shared"
	"github.com/matrix-org/gomatrixserverlib"
)

const (
	testOrigin      = gomatrixserverlib.ServerName("kaer.morhen")
	testDestination = gomatrixserverlib.ServerName("white.orchard")
)

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
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	if err != nil {
		t.Fatalf("Invalid userID: %s", err.Error())
	}
	_, request := createFederationRequest(*userID)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(httpReq, &request, &relayAPI, "1", *userID)

	expected := 200
	if response.Code != expected {
		t.Fatalf("Expected: %v, Actual: %v", expected, response.Code)
	}
}

func TestUniqueTransactionStoredInDatabase(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	if err != nil {
		t.Fatalf("Invalid userID: %s", err.Error())
	}
	inputTransaction, request := createFederationRequest(*userID)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(
		httpReq, &request, &relayAPI, inputTransaction.TransactionID, *userID)
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
