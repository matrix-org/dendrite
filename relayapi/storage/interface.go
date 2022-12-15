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

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	StoreAsyncTransaction(ctx context.Context, txn gomatrixserverlib.Transaction) (*shared.Receipt, error)
	AssociateAsyncTransactionWithDestinations(ctx context.Context, destinations map[gomatrixserverlib.UserID]struct{}, transactionID gomatrixserverlib.TransactionID, receipt *shared.Receipt) error
	CleanAsyncTransactions(ctx context.Context, userID gomatrixserverlib.UserID, receipts []*shared.Receipt) error
	GetAsyncTransaction(ctx context.Context, userID gomatrixserverlib.UserID) (*gomatrixserverlib.Transaction, *shared.Receipt, error)
	GetAsyncTransactionCount(ctx context.Context, userID gomatrixserverlib.UserID) (int64, error)
}
