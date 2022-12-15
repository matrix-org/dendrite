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

package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
)

// RelayInternalAPI is used to query information from the relay server.
type RelayInternalAPI interface {
	RelayServerAPI

	PerformRelayServerSync(
		ctx context.Context,
		request *PerformRelayServerSyncRequest,
		response *PerformRelayServerSyncResponse,
	) error
}

type RelayServerAPI interface {
	// Store async transactions for forwarding to the destination at a later time.
	PerformStoreAsync(
		ctx context.Context,
		request *PerformStoreAsyncRequest,
		response *PerformStoreAsyncResponse,
	) error

	// Obtain the oldest stored transaction for the specified userID.
	QueryAsyncTransactions(
		ctx context.Context,
		request *QueryAsyncTransactionsRequest,
		response *QueryAsyncTransactionsResponse,
	) error
}

type PerformRelayServerSyncRequest struct {
	UserID      gomatrixserverlib.UserID     `json:"user_id"`
	RelayServer gomatrixserverlib.ServerName `json:"relay_name"`
}

type PerformRelayServerSyncResponse struct {
}

type QueryRelayServersRequest struct {
	Server gomatrixserverlib.ServerName
}

type QueryRelayServersResponse struct {
	RelayServers []gomatrixserverlib.ServerName
}

type PerformStoreAsyncRequest struct {
	Txn    gomatrixserverlib.Transaction `json:"transaction"`
	UserID gomatrixserverlib.UserID      `json:"user_id"`
}

type PerformStoreAsyncResponse struct {
}

type QueryAsyncTransactionsRequest struct {
	UserID gomatrixserverlib.UserID `json:"user_id"`
}

type QueryAsyncTransactionsResponse struct {
	Txn            gomatrixserverlib.Transaction `json:"transaction"`
	RemainingCount uint32                        `json:"remaining"`
}
