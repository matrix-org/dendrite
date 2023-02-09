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

	// Retrieve from external relay server all transactions stored for us and process them.
	PerformRelayServerSync(
		ctx context.Context,
		userID gomatrixserverlib.UserID,
		relayServer gomatrixserverlib.ServerName,
	) error

	// Tells the relayapi whether or not it should act as a relay server for external servers.
	SetRelayingEnabled(bool)

	// Obtain whether the relayapi is currently configured to act as a relay server for external servers.
	RelayingEnabled() bool
}

// RelayServerAPI exposes the store & query transaction functionality of a relay server.
type RelayServerAPI interface {
	// Store transactions for forwarding to the destination at a later time.
	PerformStoreTransaction(
		ctx context.Context,
		transaction gomatrixserverlib.Transaction,
		userID gomatrixserverlib.UserID,
	) error

	// Obtain the oldest stored transaction for the specified userID.
	QueryTransactions(
		ctx context.Context,
		userID gomatrixserverlib.UserID,
		previousEntry gomatrixserverlib.RelayEntry,
	) (QueryRelayTransactionsResponse, error)
}

type QueryRelayTransactionsResponse struct {
	Transaction   gomatrixserverlib.Transaction `json:"transaction"`
	EntryID       int64                         `json:"entry_id"`
	EntriesQueued bool                          `json:"entries_queued"`
}
