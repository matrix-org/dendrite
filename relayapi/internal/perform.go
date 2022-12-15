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
	asyncResponse, err := r.fedClient.GetAsyncEvents(ctx, request.UserID, request.RelayServer)
	if err != nil {
		logrus.Errorf("GetAsyncEvents: %s", err.Error())
		return err
	}
	r.processTransaction(&asyncResponse.Transaction)

	for asyncResponse.Remaining > 0 {
		asyncResponse, err := r.fedClient.GetAsyncEvents(ctx, request.UserID, request.RelayServer)
		if err != nil {
			logrus.Errorf("GetAsyncEvents: %s", err.Error())
			return err
		}
		r.processTransaction(&asyncResponse.Transaction)
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
	logrus.Warnf("Obtaining transaction for %v", request.UserID)
	transaction, receipt, err := r.db.GetAsyncTransaction(ctx, request.UserID)
	if err != nil {
		return err
	}

	// TODO : Shouldn't be deleting unless the transaction was successfully returned...
	// TODO : Should delete transaction json from table if no more associations
	// Maybe track last received transaction, and send that as part of the request,
	// then delete before getting the new events from the db.
	if transaction != nil && receipt != nil {
		err = r.db.CleanAsyncTransactions(ctx, request.UserID, []*shared.Receipt{receipt})
		if err != nil {
			return err
		}

		// TODO : Clean async transactions json
	}

	// TODO : These db calls should happen at the same time right?
	count, err := r.db.GetAsyncTransactionCount(ctx, request.UserID)
	if err != nil {
		return err
	}

	response.RemainingCount = uint32(count)
	if transaction != nil {
		response.Txn = *transaction
		logrus.Warnf("Obtained transaction: %v", transaction.TransactionID)
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
