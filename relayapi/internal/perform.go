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

package internal

import (
	"context"

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// PerformRelayServerSync implements api.FederationInternalAPI
func (r *RelayInternalAPI) PerformRelayServerSync(
	ctx context.Context,
	request *api.PerformRelayServerSyncRequest,
	response *api.PerformRelayServerSyncResponse,
) error {
	prevEntry := gomatrixserverlib.RelayEntry{EntryID: -1}
	asyncResponse, err := r.fedClient.GetAsyncEvents(ctx, request.UserID, prevEntry, request.RelayServer)
	if err != nil {
		logrus.Errorf("GetAsyncEvents: %s", err.Error())
		return err
	}
	r.processTransaction(&asyncResponse.Txn)

	for asyncResponse.EntriesQueued {
		logrus.Info("Retrieving next entry from relay")
		logrus.Infof("Previous entry: %v", prevEntry)
		asyncResponse, err = r.fedClient.GetAsyncEvents(ctx, request.UserID, prevEntry, request.RelayServer)
		prevEntry = gomatrixserverlib.RelayEntry{EntryID: asyncResponse.EntryID}
		logrus.Infof("New previous entry: %v", prevEntry)
		if err != nil {
			logrus.Errorf("GetAsyncEvents: %s", err.Error())
			return err
		}
		r.processTransaction(&asyncResponse.Txn)
	}

	return nil
}

// PerformStoreAsync implements api.RelayInternalAPI
func (r *RelayInternalAPI) PerformStoreAsync(
	ctx context.Context,
	request *api.PerformStoreAsyncRequest,
	response *api.PerformStoreAsyncResponse,
) error {
	logrus.Warnf("Storing transaction for %v", request.UserID)
	receipt, err := r.db.StoreAsyncTransaction(ctx, request.Txn)
	if err != nil {
		logrus.Errorf("db.StoreAsyncTransaction: %s", err.Error())
		return err
	}
	err = r.db.AssociateAsyncTransactionWithDestinations(
		ctx,
		map[gomatrixserverlib.UserID]struct{}{
			request.UserID: {},
		},
		request.Txn.TransactionID,
		receipt)

	return err
}

// QueryAsyncTransactions implements api.RelayInternalAPI
func (r *RelayInternalAPI) QueryAsyncTransactions(
	ctx context.Context,
	request *api.QueryAsyncTransactionsRequest,
	response *api.QueryAsyncTransactionsResponse,
) error {
	logrus.Infof("QueryAsyncTransactions for %s", request.UserID.Raw())
	if request.PreviousEntry.EntryID >= 0 {
		logrus.Infof("Cleaning previous entry (%v) from db for %s",
			request.PreviousEntry.EntryID,
			request.UserID.Raw(),
		)
		prevReceipt := shared.NewReceipt(request.PreviousEntry.EntryID)
		err := r.db.CleanAsyncTransactions(ctx, request.UserID, []*shared.Receipt{&prevReceipt})
		if err != nil {
			logrus.Errorf("db.CleanAsyncTransactions: %s", err.Error())
			return err
		}
	}

	transaction, receipt, err := r.db.GetAsyncTransaction(ctx, request.UserID)
	if err != nil {
		logrus.Errorf("db.GetAsyncTransaction: %s", err.Error())
		return err
	}

	if transaction != nil && receipt != nil {
		logrus.Infof("Obtained transaction (%v) for %s", transaction.TransactionID, request.UserID.Raw())
		response.Txn = *transaction
		response.EntryID = receipt.GetNID()
		response.EntriesQueued = true
	} else {
		logrus.Infof("No more entries in the queue for %s", request.UserID.Raw())
		response.EntryID = -1
		response.EntriesQueued = false
	}

	return nil
}

func (r *RelayInternalAPI) processTransaction(txn *gomatrixserverlib.Transaction) {
	logrus.Warn("Processing transaction from relay server")
	mu := internal.NewMutexByRoom()
	t := internal.NewTxnReq(
		r.rsAPI,
		nil,
		r.serverName,
		r.keyRing,
		mu,
		r.producer,
		r.presenceEnabledInbound,
		txn.PDUs,
		txn.EDUs,
		txn.Origin,
		txn.TransactionID,
		txn.Destination)

	t.ProcessTransaction(context.TODO())
}
