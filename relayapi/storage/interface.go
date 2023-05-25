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

package storage

import (
	"context"

	"github.com/matrix-org/dendrite/federationapi/storage/shared/receipt"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Database interface {
	// Adds a new transaction to the queue json table.
	// Adding a duplicate transaction will result in a new row being added and a new unique nid.
	// return: unique nid representing this entry.
	StoreTransaction(ctx context.Context, txn gomatrixserverlib.Transaction) (*receipt.Receipt, error)

	// Adds a new transaction_id: server_name mapping with associated json table nid to the queue
	// entry table for each provided destination.
	AssociateTransactionWithDestinations(ctx context.Context, destinations map[spec.UserID]struct{}, transactionID gomatrixserverlib.TransactionID, dbReceipt *receipt.Receipt) error

	// Removes every server_name: receipt pair provided from the queue entries table.
	// Will then remove every entry for each receipt provided from the queue json table.
	// If any of the entries don't exist in either table, nothing will happen for that entry and
	// an error will not be generated.
	CleanTransactions(ctx context.Context, userID spec.UserID, receipts []*receipt.Receipt) error

	// Gets the oldest transaction for the provided server_name.
	// If no transactions exist, returns nil and no error.
	GetTransaction(ctx context.Context, userID spec.UserID) (*gomatrixserverlib.Transaction, *receipt.Receipt, error)

	// Gets the number of transactions being stored for the provided server_name.
	// If the server doesn't exist in the database then 0 is returned with no error.
	GetTransactionCount(ctx context.Context, userID spec.UserID) (int64, error)
}
