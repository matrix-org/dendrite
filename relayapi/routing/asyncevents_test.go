package routing_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/internal"
	"github.com/matrix-org/dendrite/relayapi/routing"
	"github.com/matrix-org/dendrite/relayapi/storage"
	"github.com/matrix-org/dendrite/relayapi/storage/shared"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func createAsyncQuery(
	userID gomatrixserverlib.UserID,
	prevEntry gomatrixserverlib.RelayEntry,
) gomatrixserverlib.FederationRequest {
	var federationPathPrefixV1 = "/_matrix/federation/v1"
	path := federationPathPrefixV1 + "/async_events/" + userID.Raw()
	request := gomatrixserverlib.NewFederationRequest("GET", userID.Domain(), "relay", path)
	request.SetContent(prevEntry)

	return request
}

func TestGetAsyncEmptyDatabaseReturnsNothing(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	transaction := createTransaction()

	_, err = db.StoreAsyncTransaction(context.Background(), transaction)
	assert.Nil(t, err, "Failed to store transaction")

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	request := createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: -1})
	response := routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, false, jsonResponse.EntriesQueued)
	assert.Equal(t, gomatrixserverlib.Transaction{}, jsonResponse.Txn)

	count, err := db.GetAsyncTransactionCount(context.Background(), *userID)
	assert.Nil(t, err)
	assert.Zero(t, count)
}

func TestGetAsyncReturnsSavedTransaction(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	transaction := createTransaction()
	receipt, err := db.StoreAsyncTransaction(context.Background(), transaction)
	assert.Nil(t, err, "Failed to store transaction")

	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction.TransactionID,
		receipt)
	assert.Nil(t, err, "Failed to associate transaction with user")

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	request := createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: -1})
	response := routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.True(t, jsonResponse.EntriesQueued)
	assert.Equal(t, transaction, jsonResponse.Txn)

	// And once more to clear the queue
	request = createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: jsonResponse.EntryID})
	response = routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse = response.JSON.(routing.AsyncEventsResponse)
	assert.False(t, jsonResponse.EntriesQueued)
	assert.Equal(t, gomatrixserverlib.Transaction{}, jsonResponse.Txn)

	count, err := db.GetAsyncTransactionCount(context.Background(), *userID)
	assert.Nil(t, err)
	assert.Zero(t, count)
}

func TestGetAsyncReturnsMultipleSavedTransactions(t *testing.T) {
	testDB := storage.NewFakeRelayDatabase()
	db := shared.Database{
		Writer:         sqlutil.NewDummyWriter(),
		RelayQueue:     testDB,
		RelayQueueJSON: testDB,
	}
	httpReq := &http.Request{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	assert.Nil(t, err, "Invalid userID")

	transaction := createTransaction()
	receipt, err := db.StoreAsyncTransaction(context.Background(), transaction)
	assert.Nil(t, err, "Failed to store transaction")

	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction.TransactionID,
		receipt)
	assert.Nil(t, err, "Failed to associate transaction with user")

	transaction2 := createTransaction()
	receipt2, err := db.StoreAsyncTransaction(context.Background(), transaction2)
	assert.Nil(t, err, "Failed to store transaction")

	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction2.TransactionID,
		receipt2)
	assert.Nil(t, err, "Failed to associate transaction with user")

	relayAPI := internal.NewRelayInternalAPI(
		&db, nil, nil, nil, nil, false, "",
	)

	request := createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: -1})
	response := routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.True(t, jsonResponse.EntriesQueued)
	assert.Equal(t, transaction, jsonResponse.Txn)

	request = createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: jsonResponse.EntryID})
	response = routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse = response.JSON.(routing.AsyncEventsResponse)
	assert.True(t, jsonResponse.EntriesQueued)
	assert.Equal(t, transaction2, jsonResponse.Txn)

	// And once more to clear the queue
	request = createAsyncQuery(*userID, gomatrixserverlib.RelayEntry{EntryID: jsonResponse.EntryID})
	response = routing.GetAsyncEvents(httpReq, &request, &relayAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse = response.JSON.(routing.AsyncEventsResponse)
	assert.False(t, jsonResponse.EntriesQueued)
	assert.Equal(t, gomatrixserverlib.Transaction{}, jsonResponse.Txn)

	count, err := db.GetAsyncTransactionCount(context.Background(), *userID)
	assert.Nil(t, err)
	assert.Zero(t, count)
}
