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
	asyncResponse, err := r.fedClient.P2PGetTransactionFromRelay(ctx, request.UserID, prevEntry, request.RelayServer)
	if err != nil {
		logrus.Errorf("P2PGetTransactionFromRelay: %s", err.Error())
		return err
	}
	r.processTransaction(&asyncResponse.Txn)

	for asyncResponse.EntriesQueued {
		logrus.Infof("Retrieving next entry from relay, previous: %v", prevEntry)
		asyncResponse, err = r.fedClient.P2PGetTransactionFromRelay(ctx, request.UserID, prevEntry, request.RelayServer)
		prevEntry = gomatrixserverlib.RelayEntry{EntryID: asyncResponse.EntryID}
		if err != nil {
			logrus.Errorf("P2PGetTransactionFromRelay: %s", err.Error())
			return err
		}
		r.processTransaction(&asyncResponse.Txn)
	}

	return nil
}

// PerformStoreTransaction implements api.RelayInternalAPI
func (r *RelayInternalAPI) PerformStoreTransaction(
	ctx context.Context,
	request *api.PerformStoreTransactionRequest,
	response *api.PerformStoreTransactionResponse,
) error {
	logrus.Warnf("Storing transaction for %v", request.UserID)
	receipt, err := r.db.StoreTransaction(ctx, request.Txn)
	if err != nil {
		logrus.Errorf("db.StoreTransaction: %s", err.Error())
		return err
	}
	err = r.db.AssociateTransactionWithDestinations(
		ctx,
		map[gomatrixserverlib.UserID]struct{}{
			request.UserID: {},
		},
		request.Txn.TransactionID,
		receipt)

	return err
}

// QueryTransactions implements api.RelayInternalAPI
func (r *RelayInternalAPI) QueryTransactions(
	ctx context.Context,
	request *api.QueryRelayTransactionsRequest,
	response *api.QueryRelayTransactionsResponse,
) error {
	logrus.Infof("QueryTransactions for %s", request.UserID.Raw())
	if request.PreviousEntry.EntryID >= 0 {
		logrus.Infof("Cleaning previous entry (%v) from db for %s",
			request.PreviousEntry.EntryID,
			request.UserID.Raw(),
		)
		prevReceipt := shared.NewReceipt(request.PreviousEntry.EntryID)
		err := r.db.CleanTransactions(ctx, request.UserID, []*shared.Receipt{&prevReceipt})
		if err != nil {
			logrus.Errorf("db.CleanTransactions: %s", err.Error())
			return err
		}
	}

	transaction, receipt, err := r.db.GetTransaction(ctx, request.UserID)
	if err != nil {
		logrus.Errorf("db.GetTransaction: %s", err.Error())
		return err
	}

	if transaction != nil && receipt != nil {
		logrus.Infof("Obtained transaction (%v) for %s", transaction.TransactionID, request.UserID.Raw())
		response.Transaction = *transaction
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
