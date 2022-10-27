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

	"github.com/matrix-org/gomatrixserverlib"
)

// AssociatePDUWithDestination creates an association that the
// destination queues will use to determine which JSON blobs to send
// to which servers.
func (d *Database) AssociatePDUWithDestinations(
	ctx context.Context,
	destinations map[gomatrixserverlib.ServerName]struct{},
	receipt *Receipt,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		var err error
		for destination := range destinations {
			err = d.FederationQueuePDUs.InsertQueuePDU(
				ctx,         // context
				txn,         // SQL transaction
				"",          // transaction ID
				destination, // destination server name
				receipt.nid, // NID from the federationapi_queue_json table
			)
		}
		return err
	})
}

// GetNextTransactionPDUs retrieves events from the database for
// the next pending transaction, up to the limit specified.
func (d *Database) GetPendingPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (
	events map[*Receipt]*gomatrixserverlib.HeaderedEvent,
	err error,
) {
	// Strictly speaking this doesn't need to be using the writer
	// since we are only performing selects, but since we don't have
	// a guarantee of transactional isolation, it's actually useful
	// to know in SQLite mode that nothing else is trying to modify
	// the database.
	events = make(map[*Receipt]*gomatrixserverlib.HeaderedEvent)
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nids, err := d.FederationQueuePDUs.SelectQueuePDUs(ctx, txn, serverName, limit)
		if err != nil {
			return fmt.Errorf("SelectQueuePDUs: %w", err)
		}

		retrieve := make([]int64, 0, len(nids))
		for _, nid := range nids {
			if event, ok := d.Cache.GetFederationQueuedPDU(nid); ok {
				events[&Receipt{nid}] = event
			} else {
				retrieve = append(retrieve, nid)
			}
		}

		blobs, err := d.FederationQueueJSON.SelectQueueJSON(ctx, txn, retrieve)
		if err != nil {
			return fmt.Errorf("SelectQueueJSON: %w", err)
		}

		for nid, blob := range blobs {
			var event gomatrixserverlib.HeaderedEvent
			if err := json.Unmarshal(blob, &event); err != nil {
				return fmt.Errorf("json.Unmarshal: %w", err)
			}
			events[&Receipt{nid}] = &event
			d.Cache.StoreFederationQueuedPDU(nid, &event)
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
	receipts []*Receipt,
) error {
	if len(receipts) == 0 {
		return errors.New("expected receipt")
	}

	nids := make([]int64, len(receipts))
	for i := range receipts {
		nids[i] = receipts[i].nid
	}

	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.FederationQueuePDUs.DeleteQueuePDUs(ctx, txn, serverName, nids); err != nil {
			return err
		}

		var deleteNIDs []int64
		for _, nid := range nids {
			count, err := d.FederationQueuePDUs.SelectQueuePDUReferenceJSONCount(ctx, txn, nid)
			if err != nil {
				return fmt.Errorf("SelectQueuePDUReferenceJSONCount: %w", err)
			}
			if count == 0 {
				deleteNIDs = append(deleteNIDs, nid)
				d.Cache.EvictFederationQueuedPDU(nid)
			}
		}

		if len(deleteNIDs) > 0 {
			if err := d.FederationQueueJSON.DeleteQueueJSON(ctx, txn, deleteNIDs); err != nil {
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
	return d.FederationQueuePDUs.SelectQueuePDUCount(ctx, nil, serverName)
}

// GetPendingServerNames returns the server names that have PDUs
// waiting to be sent.
func (d *Database) GetPendingPDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationQueuePDUs.SelectQueuePDUServerNames(ctx, nil)
}
