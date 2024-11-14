// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/element-hq/dendrite/federationapi/storage/shared/receipt"
	"github.com/element-hq/dendrite/internal/caching"
	"github.com/element-hq/dendrite/internal/sqlutil"
	"github.com/element-hq/dendrite/relayapi/storage/tables"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Database struct {
	DB                *sql.DB
	IsLocalServerName func(spec.ServerName) bool
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

	newReceipt := receipt.NewReceipt(nid)
	return &newReceipt, nil
}

func (d *Database) AssociateTransactionWithDestinations(
	ctx context.Context,
	destinations map[spec.UserID]struct{},
	transactionID gomatrixserverlib.TransactionID,
	dbReceipt *receipt.Receipt,
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
				dbReceipt.GetNID(),
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
	userID spec.UserID,
	receipts []*receipt.Receipt,
) error {
	nids := make([]int64, len(receipts))
	for i, dbReceipt := range receipts {
		nids[i] = dbReceipt.GetNID()
	}

	err := d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		deleteEntryErr := d.RelayQueue.DeleteQueueEntries(ctx, txn, userID.Domain(), nids)
		// TODO : If there are still queue entries for any of these nids for other destinations
		// then we shouldn't delete the json entries.
		// But this can't happen with the current api design.
		// There will only ever be one server entry for each nid since each call to send_relay
		// only accepts a single server name and inside there we create a new json entry.
		// So for multiple destinations we would call send_relay multiple times and have multiple
		// json entries of the same transaction.
		//
		// TLDR; this works as expected right now but can easily be optimised in the future.
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
	userID spec.UserID,
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

	newReceipt := receipt.NewReceipt(firstNID)
	return transaction, &newReceipt, nil
}

func (d *Database) GetTransactionCount(
	ctx context.Context,
	userID spec.UserID,
) (int64, error) {
	count, err := d.RelayQueue.SelectQueueEntryCount(ctx, nil, userID.Domain())
	if err != nil {
		return 0, fmt.Errorf("d.SelectQueueEntryCount: %w", err)
	}
	return count, nil
}
