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

package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/gomatrixserverlib"
)

type RelayQueue interface {
	InsertQueueEntry(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error
	DeleteQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error
	SelectQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error)
	SelectQueueEntryCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
}

type RelayQueueJSON interface {
	InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error)
	DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []int64) error
	SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error)
}
