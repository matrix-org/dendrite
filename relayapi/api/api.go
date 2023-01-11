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
	// Store transactions for forwarding to the destination at a later time.
	PerformStoreTransaction(
		ctx context.Context,
		request *PerformStoreTransactionRequest,
		response *PerformStoreTransactionResponse,
	) error

	// Obtain the oldest stored transaction for the specified userID.
	QueryTransactions(
		ctx context.Context,
		request *QueryRelayTransactionsRequest,
		response *QueryRelayTransactionsResponse,
	) error
}

type PerformRelayServerSyncRequest struct {
	UserID      gomatrixserverlib.UserID     `json:"user_id"`
	RelayServer gomatrixserverlib.ServerName `json:"relay_server"`
}

type PerformRelayServerSyncResponse struct {
}

type QueryRelayServersRequest struct {
	Server gomatrixserverlib.ServerName
}

type QueryRelayServersResponse struct {
	RelayServers []gomatrixserverlib.ServerName
}

type PerformStoreTransactionRequest struct {
	Txn    gomatrixserverlib.Transaction `json:"transaction"`
	UserID gomatrixserverlib.UserID      `json:"user_id"`
}

type PerformStoreTransactionResponse struct {
}

type QueryRelayTransactionsRequest struct {
	UserID        gomatrixserverlib.UserID     `json:"user_id"`
	PreviousEntry gomatrixserverlib.RelayEntry `json:"prev_entry,omitempty"`
}

type QueryRelayTransactionsResponse struct {
	Transaction   gomatrixserverlib.Transaction `json:"transaction"`
	EntryID       int64                         `json:"entry_id"`
	EntriesQueued bool                          `json:"entries_queued"`
}
