// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package api

import (
	"context"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// RelayInternalAPI is used to query information from the relay server.
type RelayInternalAPI interface {
	RelayServerAPI

	// Retrieve from external relay server all transactions stored for us and process them.
	PerformRelayServerSync(
		ctx context.Context,
		userID spec.UserID,
		relayServer spec.ServerName,
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
		userID spec.UserID,
	) error

	// Obtain the oldest stored transaction for the specified userID.
	QueryTransactions(
		ctx context.Context,
		userID spec.UserID,
		previousEntry fclient.RelayEntry,
	) (QueryRelayTransactionsResponse, error)
}

type QueryRelayTransactionsResponse struct {
	Transaction   gomatrixserverlib.Transaction `json:"transaction"`
	EntryID       int64                         `json:"entry_id"`
	EntriesQueued bool                          `json:"entries_queued"`
}
