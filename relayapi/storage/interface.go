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
)

type Database interface {
	StoreTransaction(ctx context.Context, txn gomatrixserverlib.Transaction) (*receipt.Receipt, error)
	AssociateTransactionWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.UserID]struct{}, transactionID gomatrixserverlib.TransactionID, receipt *receipt.Receipt) error
	CleanTransactions(ctx context.Context, userID gomatrixserverlib.UserID, receipts []*receipt.Receipt) error
	GetTransaction(ctx context.Context, userID gomatrixserverlib.UserID) (*gomatrixserverlib.Transaction, *receipt.Receipt, error)
	GetTransactionCount(ctx context.Context, userID gomatrixserverlib.UserID) (int64, error)
}
