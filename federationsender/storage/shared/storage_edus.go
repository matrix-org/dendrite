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
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		for _, nid := range receipt.nids {
			if err := d.FederationSenderQueueEDUs.InsertQueueEDU(
				ctx,        // context
				txn,        // SQL transaction
				"",         // TODO: EDU type for coalescing
				serverName, // destination server name
				nid,        // NID from the federationsender_queue_json table
			); err != nil {
				return fmt.Errorf("InsertQueueEDU: %w", err)
			}
		}
		return nil
	})
}

// GetNextTransactionEDUs retrieves events from the database for
// the next pending transaction, up to the limit specified.
func (d *Database) GetNextTransactionEDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	limit int,
) (
	edus []*gomatrixserverlib.EDU,
	receipt *Receipt,
	err error,
) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		nids, err := d.FederationSenderQueueEDUs.SelectQueueEDUs(ctx, txn, serverName, limit)
		if err != nil {
			return fmt.Errorf("SelectQueueEDUs: %w", err)
		}

		receipt = &Receipt{
			nids: nids,
		}

		blobs, err := d.FederationSenderQueueJSON.SelectQueueJSON(ctx, txn, nids)
		if err != nil {
			return fmt.Errorf("SelectQueueJSON: %w", err)
		}

		for _, blob := range blobs {
			var event gomatrixserverlib.EDU
			if err := json.Unmarshal(blob, &event); err != nil {
				return fmt.Errorf("json.Unmarshal: %w", err)
			}
			edus = append(edus, &event)
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
	receipt *Receipt,
) error {
	if receipt == nil {
		return errors.New("expected receipt")
	}

	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		if err := d.FederationSenderQueueEDUs.DeleteQueueEDUs(ctx, txn, serverName, receipt.nids); err != nil {
			return err
		}

		var deleteNIDs []int64
		for _, nid := range receipt.nids {
			count, err := d.FederationSenderQueueEDUs.SelectQueueEDUReferenceJSONCount(ctx, txn, nid)
			if err != nil {
				return fmt.Errorf("SelectQueueEDUReferenceJSONCount: %w", err)
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

// GetPendingEDUCount returns the number of EDUs waiting to be
// sent for a given servername.
func (d *Database) GetPendingEDUCount(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
) (int64, error) {
	return d.FederationSenderQueueEDUs.SelectQueueEDUCount(ctx, nil, serverName)
}

// GetPendingServerNames returns the server names that have EDUs
// waiting to be sent.
func (d *Database) GetPendingEDUServerNames(
	ctx context.Context,
) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationSenderQueueEDUs.SelectQueueEDUServerNames(ctx, nil)
}
