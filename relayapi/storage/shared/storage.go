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

	"github.com/matrix-org/dendrite/federationapi/storage/shared"
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
	ctx context.Context, txn gomatrixserverlib.Transaction,
) (*shared.Receipt, error) {
	var err error
	json, err := json.Marshal(txn)
	if err != nil {
		return nil, fmt.Errorf("d.JSONUnmarshall: %w", err)
	}

	var nid int64
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nid, err = d.RelayQueueJSON.InsertQueueJSON(ctx, txn, string(json))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("d.insertQueueJSON: %w", err)
	}

	receipt := shared.NewReceipt(nid)
	return &receipt, nil
}

func (d *Database) AssociateTransactionWithDestinations(
	ctx context.Context,
	destinations map[gomatrixserverlib.UserID]struct{},
	transactionID gomatrixserverlib.TransactionID,
	receipt *shared.Receipt,
) error {
	for destination := range destinations {
		err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
			err := d.RelayQueue.InsertQueueEntry(
				ctx, txn, transactionID, destination.Domain(), receipt.GetNID())
			return err
		})
		if err != nil {
			return fmt.Errorf("d.insertQueueEntry: %w", err)
		}
	}

	return nil
}

func (d *Database) CleanTransactions(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
	receipts []*shared.Receipt,
) error {
	println(len(receipts))
	nids := make([]int64, len(receipts))
	for i, receipt := range receipts {
		nids[i] = receipt.GetNID()
	}

	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		err := d.RelayQueue.DeleteQueueEntries(ctx, txn, userID.Domain(), nids)
		return err
	})
	if err != nil {
		return fmt.Errorf("d.deleteQueueEntries: %w", err)
	}

	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		dbErr := d.RelayQueueJSON.DeleteQueueJSON(ctx, txn, nids)
		return dbErr
	})
	if err != nil {
		return fmt.Errorf("d.deleteQueueJSON: %w", err)
	}

	return nil
}

func (d *Database) GetTransaction(
	ctx context.Context,
	userID gomatrixserverlib.UserID,
) (*gomatrixserverlib.Transaction, *shared.Receipt, error) {
	nids, err := d.RelayQueue.SelectQueueEntries(ctx, nil, userID.Domain(), 1)
	if err != nil {
		return nil, nil, fmt.Errorf("d.SelectQueueEntries: %w", err)
	}
	if len(nids) == 0 {
		return nil, nil, nil
	}

	txns := map[int64][]byte{}
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		txns, err = d.RelayQueueJSON.SelectQueueJSON(ctx, txn, nids)
		return err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("d.SelectQueueJSON: %w", err)
	}

	transaction := &gomatrixserverlib.Transaction{}
	err = json.Unmarshal(txns[nids[0]], transaction)
	if err != nil {
		return nil, nil, fmt.Errorf("Unmarshall transaction: %w", err)
	}

	receipt := shared.NewReceipt(nids[0])
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
