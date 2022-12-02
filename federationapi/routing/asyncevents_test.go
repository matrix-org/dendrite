package routing_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/stretchr/testify/assert"
)

func TestGetAsyncEmptyDatabaseReturnsNothing(t *testing.T) {
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
	transaction := createTransaction()

	_, err = db.StoreAsyncTransaction(context.Background(), transaction)
	if err != nil {
		t.Fatalf("Failed to store transaction: %s", err.Error())
	}

	fedAPI := internal.NewFederationInternalAPI(
		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
	)

	response := routing.GetAsyncEvents(httpReq, nil, fedAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, uint32(0), jsonResponse.Remaining)
	assert.Equal(t, gomatrixserverlib.Transaction{}, jsonResponse.Transaction)
}

func TestGetAsyncReturnsSavedTransaction(t *testing.T) {
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
	transaction := createTransaction()

	receipt, err := db.StoreAsyncTransaction(context.Background(), transaction)
	if err != nil {
		t.Fatalf("Failed to store transaction: %s", err.Error())
	}
	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction.TransactionID,
		receipt)
	if err != nil {
		t.Fatalf("Failed to associate transaction with user: %s", err.Error())
	}

	fedAPI := internal.NewFederationInternalAPI(
		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
	)

	response := routing.GetAsyncEvents(httpReq, nil, fedAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, uint32(0), jsonResponse.Remaining)
	assert.Equal(t, transaction, jsonResponse.Transaction)
}

func TestGetAsyncReturnsMultipleSavedTransactions(t *testing.T) {
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

	transaction := createTransaction()
	receipt, err := db.StoreAsyncTransaction(context.Background(), transaction)
	if err != nil {
		t.Fatalf("Failed to store transaction: %s", err.Error())
	}
	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction.TransactionID,
		receipt)
	if err != nil {
		t.Fatalf("Failed to associate transaction with user: %s", err.Error())
	}

	transaction2 := createTransaction()
	receipt2, err := db.StoreAsyncTransaction(context.Background(), transaction2)
	if err != nil {
		t.Fatalf("Failed to store transaction: %s", err.Error())
	}
	err = db.AssociateAsyncTransactionWithDestinations(
		context.Background(),
		map[gomatrixserverlib.UserID]struct{}{
			*userID: {},
		},
		transaction2.TransactionID,
		receipt2)
	if err != nil {
		t.Fatalf("Failed to associate transaction with user: %s", err.Error())
	}

	fedAPI := internal.NewFederationInternalAPI(
		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
	)

	response := routing.GetAsyncEvents(httpReq, nil, fedAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, uint32(1), jsonResponse.Remaining)
	assert.Equal(t, transaction, jsonResponse.Transaction)

	response = routing.GetAsyncEvents(httpReq, nil, fedAPI, *userID)
	assert.Equal(t, http.StatusOK, response.Code)

	jsonResponse = response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, uint32(0), jsonResponse.Remaining)
	assert.Equal(t, transaction2, jsonResponse.Transaction)
}
