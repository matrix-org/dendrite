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

package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/federationapi/storage/shared/receipt"
	"github.com/matrix-org/dendrite/internal/caching"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/relayapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                *sql.DB
	IsLocalServerName func(gomatrixserverlib.ServerName) bool
	Cache             caching.FederationCache
	Writer            sqlutil.Writer
	RelayQueue        tables.RelayQueue
	RelayQueueJSON    tables.RelayQueueJSON
}

func (d *Database) StoreTransaction(
	ctx context.Context,
	transaction gomatrixserverlib.Transaction,
) (*receipt.Receipt, error) {
	var err error
	jsonTransaction, err := json.Marshal(transaction)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal: %w", err)
	}

	var nid int64
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nid, err = d.RelayQueueJSON.InsertQueueJSON(ctx, txn, string(jsonTransaction))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("d.insertQueueJSON: %w", err)
	}

	receipt := receipt.NewReceipt(nid)
	return &receipt, nil
}

func (d *Database) AssociateTransactionWithDestinations(
	ctx context.Context,
	destinations map[gomatrixserverlib.UserID]struct{},
	transactionID gomatrixserverlib.TransactionID,
	receipt *receipt.Receipt,
) error {
	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		var lastErr error
		for destination := range destinations {
			destination := destination
			err := d.RelayQueue.InsertQueueEntry(
				ctx,
				txn,
				transactionID,
				destination.Domain(),
				receipt.GetNID(),
			)
			if err != nil {
				lastErr = fmt.Errorf("d.insertQueueEntry: %w", err)
			}
		}
		return lastErr
	})

	return err
}

func (d *Database) CleanTransactions(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
	receipts []*receipt.Receipt,
) error {
	nids := make([]int64, len(receipts))
	for i, receipt := range receipts {
		nids[i] = receipt.GetNID()
	}

	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		deleteEntryErr := d.RelayQueue.DeleteQueueEntries(ctx, txn, userID.Domain(), nids)
		deleteJSONErr := d.RelayQueueJSON.DeleteQueueJSON(ctx, txn, nids)

		if deleteEntryErr != nil {
			return fmt.Errorf("d.deleteQueueEntries: %w", deleteEntryErr)
		}
		if deleteJSONErr != nil {
			return fmt.Errorf("d.deleteQueueJSON: %w", deleteJSONErr)
		}
		return nil
	})

	return err
}

func (d *Database) GetTransaction(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
) (*gomatrixserverlib.Transaction, *receipt.Receipt, error) {
	entriesRequested := 1
	nids, err := d.RelayQueue.SelectQueueEntries(ctx, nil, userID.Domain(), entriesRequested)
	if err != nil {
		return nil, nil, fmt.Errorf("d.SelectQueueEntries: %w", err)
	}
	if len(nids) == 0 {
		return nil, nil, nil
	}
	firstNID := nids[0]

	txns := map[int64][]byte{}
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		txns, err = d.RelayQueueJSON.SelectQueueJSON(ctx, txn, nids)
		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("d.SelectQueueJSON: %w", err)
	}

	transaction := &gomatrixserverlib.Transaction{}
	if _, ok := txns[firstNID]; !ok {
		return nil, nil, fmt.Errorf("Failed retrieving json blob for transaction: %d", firstNID)
	}

	err = json.Unmarshal(txns[firstNID], transaction)
	if err != nil {
		return nil, nil, fmt.Errorf("Unmarshal transaction: %w", err)
	}

	receipt := receipt.NewReceipt(firstNID)
	return transaction, &receipt, nil
}

func (d *Database) GetTransactionCount(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
) (int64, error) {
	count, err := d.RelayQueue.SelectQueueEntryCount(ctx, nil, userID.Domain())
	if err != nil {
		return 0, fmt.Errorf("d.SelectQueueEntryCount: %w", err)
	}
	return count, nil
}
