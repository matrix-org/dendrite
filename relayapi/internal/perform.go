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

	"github.com/matrix-org/dendrite/federationapi/storage/shared/receipt"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/relayapi/api"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/sirupsen/logrus"
)

// SetRelayingEnabled implements api.RelayInternalAPI
func (r *RelayInternalAPI) SetRelayingEnabled(enabled bool) {
	r.relayingEnabledMutex.Lock()
	defer r.relayingEnabledMutex.Unlock()
	r.relayingEnabled = enabled
}

// RelayingEnabled implements api.RelayInternalAPI
func (r *RelayInternalAPI) RelayingEnabled() bool {
	r.relayingEnabledMutex.Lock()
	defer r.relayingEnabledMutex.Unlock()
	return r.relayingEnabled
}

// PerformRelayServerSync implements api.RelayInternalAPI
func (r *RelayInternalAPI) PerformRelayServerSync(
	ctx context.Context,
	userID spec.UserID,
	relayServer spec.ServerName,
) error {
	// Providing a default RelayEntry (EntryID = 0) is done to ask the relay if there are any
	// transactions available for this node.
	prevEntry := fclient.RelayEntry{}
	asyncResponse, err := r.fedClient.P2PGetTransactionFromRelay(ctx, userID, prevEntry, relayServer)
	if err != nil {
		logrus.Errorf("P2PGetTransactionFromRelay: %s", err.Error())
		return err
	}
	r.processTransaction(&asyncResponse.Transaction)

	prevEntry = fclient.RelayEntry{EntryID: asyncResponse.EntryID}
	for asyncResponse.EntriesQueued {
		// There are still more entries available for this node from the relay.
		logrus.Infof("Retrieving next entry from relay, previous: %v", prevEntry)
		asyncResponse, err = r.fedClient.P2PGetTransactionFromRelay(ctx, userID, prevEntry, relayServer)
		prevEntry = fclient.RelayEntry{EntryID: asyncResponse.EntryID}
		if err != nil {
			logrus.Errorf("P2PGetTransactionFromRelay: %s", err.Error())
			return err
		}
		r.processTransaction(&asyncResponse.Transaction)
	}

	return nil
}

// PerformStoreTransaction implements api.RelayInternalAPI
func (r *RelayInternalAPI) PerformStoreTransaction(
	ctx context.Context,
	transaction gomatrixserverlib.Transaction,
	userID spec.UserID,
) error {
	logrus.Warnf("Storing transaction for %v", userID)
	receipt, err := r.db.StoreTransaction(ctx, transaction)
	if err != nil {
		logrus.Errorf("db.StoreTransaction: %s", err.Error())
		return err
	}
	err = r.db.AssociateTransactionWithDestinations(
		ctx,
		map[spec.UserID]struct{}{
			userID: {},
		},
		transaction.TransactionID,
		receipt)

	return err
}

// QueryTransactions implements api.RelayInternalAPI
func (r *RelayInternalAPI) QueryTransactions(
	ctx context.Context,
	userID spec.UserID,
	previousEntry fclient.RelayEntry,
) (api.QueryRelayTransactionsResponse, error) {
	logrus.Infof("QueryTransactions for %s", userID.Raw())
	if previousEntry.EntryID > 0 {
		logrus.Infof("Cleaning previous entry (%v) from db for %s",
			previousEntry.EntryID,
			userID.Raw(),
		)
		prevReceipt := receipt.NewReceipt(previousEntry.EntryID)
		err := r.db.CleanTransactions(ctx, userID, []*receipt.Receipt{&prevReceipt})
		if err != nil {
			logrus.Errorf("db.CleanTransactions: %s", err.Error())
			return api.QueryRelayTransactionsResponse{}, err
		}
	}

	transaction, receipt, err := r.db.GetTransaction(ctx, userID)
	if err != nil {
		logrus.Errorf("db.GetTransaction: %s", err.Error())
		return api.QueryRelayTransactionsResponse{}, err
	}

	response := api.QueryRelayTransactionsResponse{}
	if transaction != nil && receipt != nil {
		logrus.Infof("Obtained transaction (%v) for %s", transaction.TransactionID, userID.Raw())
		response.Transaction = *transaction
		response.EntryID = receipt.GetNID()
		response.EntriesQueued = true
	} else {
		logrus.Infof("No more entries in the queue for %s", userID.Raw())
		response.EntryID = 0
		response.EntriesQueued = false
	}

	return response, nil
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
