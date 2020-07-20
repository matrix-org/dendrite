// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"errors"
	"fmt"

	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

// AssociatePDUWithDestination creates an association that the
// destination queues will use to determine which JSON blobs to send
// to which servers.
func (d *Database) AssociatePDUWithDestination(
	ctx context.Context,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	receipt *Receipt,
) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		for _, nid := range receipt.nids {
			if err := d.FederationSenderQueuePDUs.InsertQueuePDU(
				ctx,           // context
				txn,           // SQL transaction
				transactionID, // transaction ID
				serverName,    // destination server name
				nid,           // NID from the federationsender_queue_json table
			); err != nil {
				return fmt.Errorf("InsertQueuePDU: %w", err)
			}
		}
		return nil
	})
}

// GetNextTransactionPDUs retrieves events from the database for
// the next pending transaction, up to the limit specified.
func (d *Database) GetNextTransactionPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (
	transactionID gomatrixserverlib.TransactionID,
	events []*gomatrixserverlib.HeaderedEvent,
	receipt *Receipt,
	err error,
) {
	err = sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		transactionID, err = d.FederationSenderQueuePDUs.SelectQueuePDUNextTransactionID(ctx, txn, serverName)
		if err != nil {
			return fmt.Errorf("SelectQueuePDUNextTransactionID: %w", err)
		}

		if transactionID == "" {
			return nil
		}

		nids, err := d.FederationSenderQueuePDUs.SelectQueuePDUs(ctx, txn, serverName, transactionID, limit)
		if err != nil {
			return fmt.Errorf("SelectQueuePDUs: %w", err)
		}

		receipt = &Receipt{
			nids: nids,
		}

		blobs, err := d.FederationSenderQueueJSON.SelectQueueJSON(ctx, txn, nids)
		if err != nil {
			return fmt.Errorf("SelectQueueJSON: %w", err)
		}

		for _, blob := range blobs {
			var event gomatrixserverlib.HeaderedEvent
			if err := json.Unmarshal(blob, &event); err != nil {
				return fmt.Errorf("json.Unmarshal: %w", err)
			}
			events = append(events, &event)
		}

		return nil
	})
	return
}

// CleanTransactionPDUs cleans up all associated events for a
// given transaction. This is done when the transaction was sent
// successfully.
func (d *Database) CleanPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	receipt *Receipt,
) error {
	if receipt == nil {
		return errors.New("expected receipt")
	}

	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		if err := d.FederationSenderQueuePDUs.DeleteQueuePDUs(ctx, txn, serverName, receipt.nids); err != nil {
			return err
		}

		var deleteNIDs []int64
		for _, nid := range receipt.nids {
			count, err := d.FederationSenderQueuePDUs.SelectQueuePDUReferenceJSONCount(ctx, txn, nid)
			if err != nil {
				return fmt.Errorf("SelectQueuePDUReferenceJSONCount: %w", err)
			}
			if count == 0 {
				deleteNIDs = append(deleteNIDs, nid)
			}
		}

		if len(deleteNIDs) > 0 {
			if err := d.FederationSenderQueueJSON.DeleteQueueJSON(ctx, txn, deleteNIDs); err != nil {
				return fmt.Errorf("DeleteQueueJSON: %w", err)
			}
		}

		return nil
	})
}

// GetPendingPDUCount returns the number of PDUs waiting to be
// sent for a given servername.
func (d *Database) GetPendingPDUCount(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (int64, error) {
	return d.FederationSenderQueuePDUs.SelectQueuePDUCount(ctx, nil, serverName)
}

// GetPendingServerNames returns the server names that have PDUs
// waiting to be sent.
func (d *Database) GetPendingPDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationSenderQueuePDUs.SelectQueuePDUServerNames(ctx, nil)
}
