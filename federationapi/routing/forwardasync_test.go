package routing_test

import (
	// "context"
	"net/http"
	"testing"

	"github.com/matrix-org/dendrite/federationapi/internal"
	"github.com/matrix-org/dendrite/federationapi/routing"
	// "github.com/matrix-org/dendrite/federationapi/storage/shared"
	// "github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/gomatrixserverlib"
)

func TestEmptyForwardReturnsOk(t *testing.T) {
	httpReq := &http.Request{}
	request := &gomatrixserverlib.FederationRequest{}
	fedAPI := internal.FederationInternalAPI{}
	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
	if err != nil {
		t.Fatalf("Invalid userID: %s", err.Error())
	}

	response := routing.ForwardAsync(httpReq, request, &fedAPI, "1", *userID)

	expected := 200
	if response.Code != expected {
		t.Fatalf("Expected: %v, Actual: %v", expected, response.Code)
	}
}

// func TestUniqueTransactionStoredInDatabase(t *testing.T) {
// 	db := shared.Database{}
// 	httpReq := &http.Request{}
// 	inputTransaction := gomatrixserverlib.Transaction{}
// 	request := &gomatrixserverlib.FederationRequest{}
// 	fedAPI := internal.NewFederationInternalAPI(
// 		&db, &config.FederationAPI{}, nil, nil, nil, nil, nil, nil,
// 	)
// 	userID, err := gomatrixserverlib.NewUserID("@local:domain", false)
// 	if err != nil {
// 		t.Fatalf("Invalid userID: %s", err.Error())
// 	}

// 	response := routing.ForwardAsync(httpReq, request, fedAPI, "1", *userID)
// 	transaction, err := db.GetAsyncTransaction(context.TODO(), *userID)
// 	transactionCount, err := db.GetAsyncTransactionCount(context.TODO(), *userID)

// 	expected := 200
// 	if response.Code != expected {
// 		t.Fatalf("Expected Return Code: %v, Actual: %v", expected, response.Code)
// 	}
// 	if transactionCount != 1 {
// 		t.Fatalf("Expected count of 1, Actual: %d", transactionCount)
// 	}
// 	if transaction.TransactionID != inputTransaction.TransactionID {
// 		t.Fatalf("Expected Transaction ID: %s, Actual: %s",
// 			inputTransaction.TransactionID, transaction.TransactionID)
// 	}
// }

// func TestDuplicateTransactionNotStoredInDatabase(t *testing.T) {

// }