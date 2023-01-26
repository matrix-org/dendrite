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

// RelayQueue table contains a mapping of server name to transaction id and the corresponding nid.
// These are the transactions being stored for the given destination server.
// The nids correspond to entries in the RelayQueueJSON table.
type RelayQueue interface {
	// Adds a new transaction_id: server_name mapping with associated json table nid to the table.
	// Will ensure only one transaction id is present for each server_name: nid mapping.
	// Adding duplicates will silently do nothing.
	InsertQueueEntry(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error

	// Removes multiple entries from the table corresponding the the list of nids provided.
	// If any of the provided nids don't match a row in the table, that deletion is considered
	// successful.
	DeleteQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, jsonNIDs []int64) error

	// Get a list of nids associated with the provided server name.
	// Returns up to `limit` nids. The entries are returned oldest first.
	// Will return an empty list if no matches were found.
	SelectQueueEntries(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, limit int) ([]int64, error)

	// Get the number of entries in the table associated with the provided server name.
	// If there are no matching rows, a count of 0 is returned with err set to nil.
	SelectQueueEntryCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
}

// RelayQueueJSON table contains a map of nid to the raw transaction json.
type RelayQueueJSON interface {
	// Adds a new transaction to the table.
	// Adding a duplicate transaction will result in a new row being added and a new unique nid.
	// return: unique nid representing this entry.
	InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error)

	// Removes multiple nids from the table.
	// If any of the provided nids don't match a row in the table, that deletion is considered
	// successful.
	DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []int64) error

	// Get the transaction json corresponding to the provided nids.
	// Will return a partial result containing any matching nid from the table.
	// Will return an empty map if no matches were found.
	// It is the caller's responsibility to deal with the results appropriately.
	// return: map indexed by nid of each matching transaction json.
	SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error)
}
