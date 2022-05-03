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

// AssociateEDUWithDestination creates an association that the
// destination queues will use to determine which JSON blobs to send
// to which servers.
func (d *Database) AssociateEDUWithDestination(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	receipt *Receipt,
	eduType string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.FederationQueueEDUs.InsertQueueEDU(
			ctx,         // context
			txn,         // SQL transaction
			eduType,     // EDU type for coalescing
			serverName,  // destination server name
			receipt.nid, // NID from the federationapi_queue_json table
		); err != nil {
			return fmt.Errorf("InsertQueueEDU: %w", err)
		}
		return nil
	})
}

// GetNextTransactionEDUs retrieves events from the database for
// the next pending transaction, up to the limit specified.
func (d *Database) GetPendingEDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (
	edus map[*Receipt]*gomatrixserverlib.EDU,
	err error,
) {
	edus = make(map[*Receipt]*gomatrixserverlib.EDU)
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nids, err := d.FederationQueueEDUs.SelectQueueEDUs(ctx, txn, serverName, limit)
		if err != nil {
			return fmt.Errorf("SelectQueueEDUs: %w", err)
		}

		retrieve := make([]int64, 0, len(nids))
		for _, nid := range nids {
			if edu, ok := d.Cache.GetFederationQueuedEDU(nid); ok {
				edus[&Receipt{nid}] = edu
			} else {
				retrieve = append(retrieve, nid)
			}
		}

		blobs, err := d.FederationQueueJSON.SelectQueueJSON(ctx, txn, retrieve)
		if err != nil {
			return fmt.Errorf("SelectQueueJSON: %w", err)
		}

		for nid, blob := range blobs {
			var event gomatrixserverlib.EDU
			if err := json.Unmarshal(blob, &event); err != nil {
				return fmt.Errorf("json.Unmarshal: %w", err)
			}
			edus[&Receipt{nid}] = &event
		}

		return nil
	})
	return
}

// CleanEDUs cleans up all specified EDUs. This is done when a
// transaction was sent successfully.
func (d *Database) CleanEDUs(
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
		if err := d.FederationQueueEDUs.DeleteQueueEDUs(ctx, txn, serverName, nids); err != nil {
			return err
		}

		var deleteNIDs []int64
		for _, nid := range nids {
			count, err := d.FederationQueueEDUs.SelectQueueEDUReferenceJSONCount(ctx, txn, nid)
			if err != nil {
				return fmt.Errorf("SelectQueueEDUReferenceJSONCount: %w", err)
			}
			if count == 0 {
				deleteNIDs = append(deleteNIDs, nid)
				d.Cache.EvictFederationQueuedEDU(nid)
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

// GetPendingEDUCount returns the number of EDUs waiting to be
// sent for a given servername.
func (d *Database) GetPendingEDUCount(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (int64, error) {
	return d.FederationQueueEDUs.SelectQueueEDUCount(ctx, nil, serverName)
}

// GetPendingServerNames returns the server names that have EDUs
// waiting to be sent.
func (d *Database) GetPendingEDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationQueueEDUs.SelectQueueEDUServerNames(ctx, nil)
}
