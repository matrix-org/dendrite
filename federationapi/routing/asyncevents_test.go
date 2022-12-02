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

	response := routing.GetAsyncEvents(httpReq, fedAPI, *userID)
	assert.Equal(t, response.Code, http.StatusOK)

	jsonResponse := response.JSON.(routing.AsyncEventsResponse)
	assert.Equal(t, jsonResponse.Remaining, uint32(0))
	assert.Equal(t, jsonResponse.Transaction, gomatrixserverlib.Transaction{})
}
