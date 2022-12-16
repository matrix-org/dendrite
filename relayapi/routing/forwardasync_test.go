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
	"github.com/stretchr/testify/assert"
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

func createFederationRequest(
	userID gomatrixserverlib.UserID,
	txnID gomatrixserverlib.TransactionID,
	origin gomatrixserverlib.ServerName,
	destination gomatrixserverlib.ServerName,
	content interface{},
) gomatrixserverlib.FederationRequest {
	var federationPathPrefixV1 = "/_matrix/federation/v1"
	path := federationPathPrefixV1 + "/forward_async/" + string(txnID) + "/" + userID.Raw()
	request := gomatrixserverlib.NewFederationRequest("PUT", origin, destination, path)
	request.SetContent(content)

	return request
}

func TestForwardEmptyReturnsOk(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	txn := createTransaction()
	request := createFederationRequest(*userID, txn.TransactionID, txn.Origin, txn.Destination, txn)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(httpReq, &request, &relayAPI, "1", *userID)

	assert.Equal(t, 200, response.Code)
}

func TestForwardBadJSONReturnsError(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	type BadData struct {
		Field bool `json:"pdus"`
	}
	content := BadData{
		Field: false,
	}
	txn := createTransaction()
	request := createFederationRequest(*userID, txn.TransactionID, txn.Origin, txn.Destination, content)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(httpReq, &request, &relayAPI, "1", *userID)

	assert.NotEqual(t, 200, response.Code)
}

func TestForwardTooManyPDUsReturnsError(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	type BadData struct {
		Field []json.RawMessage `json:"pdus"`
	}
	content := BadData{
		Field: []json.RawMessage{},
	}
	for i := 0; i < 51; i++ {
		content.Field = append(content.Field, []byte{})
	}
	assert.Greater(t, len(content.Field), 50)

	txn := createTransaction()
	request := createFederationRequest(*userID, txn.TransactionID, txn.Origin, txn.Destination, content)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(httpReq, &request, &relayAPI, "1", *userID)

	assert.NotEqual(t, 200, response.Code)
}

func TestForwardTooManyEDUsReturnsError(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	type BadData struct {
		Field []gomatrixserverlib.EDU `json:"edus"`
	}
	content := BadData{
		Field: []gomatrixserverlib.EDU{},
	}
	for i := 0; i < 101; i++ {
		content.Field = append(content.Field, gomatrixserverlib.EDU{Type: gomatrixserverlib.MTyping})
	}
	assert.Greater(t, len(content.Field), 100)

	txn := createTransaction()
	request := createFederationRequest(*userID, txn.TransactionID, txn.Origin, txn.Destination, content)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(httpReq, &request, &relayAPI, "1", *userID)

	assert.NotEqual(t, 200, response.Code)
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
	assert.Nil(t, err, "Invalid userID")

	txn := createTransaction()
	request := createFederationRequest(*userID, txn.TransactionID, txn.Origin, txn.Destination, txn)

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	response := routing.ForwardAsync(
		httpReq, &request, &relayAPI, txn.TransactionID, *userID)
	transaction, _, err := db.GetAsyncTransaction(context.TODO(), *userID)
	assert.Nil(t, err, "Failed retrieving transaction")

	transactionCount, err := db.GetAsyncTransactionCount(context.TODO(), *userID)
	assert.Nil(t, err, "Failed retrieving transaction count")

	assert.Equal(t, 200, response.Code)
	assert.Equal(t, int64(1), transactionCount)
	assert.Equal(t, txn.TransactionID, transaction.TransactionID)
}
