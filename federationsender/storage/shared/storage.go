package shared

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/dendrite/federationsender/storage/tables"
	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database struct {
	DB                          *sql.DB
	FederationSenderQueuePDUs   tables.FederationSenderQueuePDUs
	FederationSenderQueueEDUs   tables.FederationSenderQueueEDUs
	FederationSenderQueueJSON   tables.FederationSenderQueueJSON
	FederationSenderJoinedHosts tables.FederationSenderJoinedHosts
	FederationSenderRooms       tables.FederationSenderRooms
}

// UpdateRoom updates the joined hosts for a room and returns what the joined
// hosts were before the update, or nil if this was a duplicate message.
// This is called when we receive a message from kafka, so we pass in
// oldEventID and newEventID to check that we haven't missed any messages or
// this isn't a duplicate message.
func (d *Database) UpdateRoom(
	ctx context.Context,
	roomID, oldEventID, newEventID string,
	addHosts []types.JoinedHost,
	removeHosts []string,
) (joinedHosts []types.JoinedHost, err error) {
	err = sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		err = d.FederationSenderRooms.InsertRoom(ctx, txn, roomID)
		if err != nil {
			return err
		}

		lastSentEventID, err := d.FederationSenderRooms.SelectRoomForUpdate(ctx, txn, roomID)
		if err != nil {
			return err
		}

		if lastSentEventID == newEventID {
			// We've handled this message before, so let's just ignore it.
			// We can only get a duplicate for the last message we processed,
			// so its enough just to compare the newEventID with lastSentEventID
			return nil
		}

		if lastSentEventID != "" && lastSentEventID != oldEventID {
			return types.EventIDMismatchError{
				DatabaseID: lastSentEventID, RoomServerID: oldEventID,
			}
		}

		joinedHosts, err = d.FederationSenderJoinedHosts.SelectJoinedHostsWithTx(ctx, txn, roomID)
		if err != nil {
			return err
		}

		for _, add := range addHosts {
			err = d.FederationSenderJoinedHosts.InsertJoinedHosts(ctx, txn, roomID, add.MemberEventID, add.ServerName)
			if err != nil {
				return err
			}
		}
		if err = d.FederationSenderJoinedHosts.DeleteJoinedHosts(ctx, txn, removeHosts); err != nil {
			return err
		}
		return d.FederationSenderRooms.UpdateRoom(ctx, txn, roomID, newEventID)
	})
	return
}

// GetJoinedHosts returns the currently joined hosts for room,
// as known to federationserver.
// Returns an error if something goes wrong.
func (d *Database) GetJoinedHosts(
	ctx context.Context, roomID string,
) ([]types.JoinedHost, error) {
	return d.FederationSenderJoinedHosts.SelectJoinedHosts(ctx, roomID)
}

// GetAllJoinedHosts returns the currently joined hosts for
// all rooms known to the federation sender.
// Returns an error if something goes wrong.
func (d *Database) GetAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error) {
	return d.FederationSenderJoinedHosts.SelectAllJoinedHosts(ctx)
}

// StoreJSON adds a JSON blob into the queue JSON table and returns
// a NID. The NID will then be used when inserting the per-destination
// metadata entries.
func (d *Database) StoreJSON(
	ctx context.Context, js string,
) (int64, error) {
	nid, err := d.FederationSenderQueueJSON.InsertQueueJSON(ctx, nil, js)
	if err != nil {
		return 0, fmt.Errorf("d.insertQueueJSON: %w", err)
	}
	return nid, nil
}

// AssociatePDUWithDestination creates an association that the
// destination queues will use to determine which JSON blobs to send
// to which servers.
func (d *Database) AssociatePDUWithDestination(
	ctx context.Context,
	transactionID gomatrixserverlib.TransactionID,
	serverName gomatrixserverlib.ServerName,
	nids []int64,
) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		for _, nid := range nids {
			if err := d.FederationSenderQueuePDUs.InsertQueuePDU(
				ctx,           // context
				txn,           // SQL transaction
				transactionID, // transaction ID
				serverName,    // destination server name
				nid,           // NID from the federationsender_queue_json table
			); err != nil {
				return fmt.Errorf("d.insertQueueRetryStmt.ExecContext: %w", err)
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
	err error,
) {
	err = sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		transactionID, err = d.FederationSenderQueuePDUs.SelectQueuePDUNextTransactionID(ctx, txn, serverName)
		if err != nil {
			return fmt.Errorf("d.selectQueueNextTransactionID: %w", err)
		}

		if transactionID == "" {
			return nil
		}

		nids, err := d.FederationSenderQueuePDUs.SelectQueuePDUs(ctx, txn, serverName, transactionID, limit)
		if err != nil {
			return fmt.Errorf("d.selectQueuePDUs: %w", err)
		}

		blobs, err := d.FederationSenderQueueJSON.SelectQueueJSON(ctx, txn, nids)
		if err != nil {
			return fmt.Errorf("d.selectJSON: %w", err)
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
func (d *Database) CleanTransactionPDUs(
	ctx context.Context,
	serverName gomatrixserverlib.ServerName,
	transactionID gomatrixserverlib.TransactionID,
) error {
	return sqlutil.WithTransaction(d.DB, func(txn *sql.Tx) error {
		nids, err := d.FederationSenderQueuePDUs.SelectQueuePDUs(ctx, txn, serverName, transactionID, 50)
		if err != nil {
			return fmt.Errorf("d.selectQueuePDUs: %w", err)
		}

		if err = d.FederationSenderQueuePDUs.DeleteQueuePDUTransaction(ctx, txn, serverName, transactionID); err != nil {
			return fmt.Errorf("d.deleteQueueTransaction: %w", err)
		}

		var count int64
		var deleteNIDs []int64
		for _, nid := range nids {
			count, err = d.FederationSenderQueuePDUs.SelectQueuePDUReferenceJSONCount(ctx, txn, nid)
			if err != nil {
				return fmt.Errorf("d.selectQueueReferenceJSONCount: %w", err)
			}
			if count == 0 {
				deleteNIDs = append(deleteNIDs, nid)
			}
		}

		if len(deleteNIDs) > 0 {
			if err = d.FederationSenderQueueJSON.DeleteQueueJSON(ctx, txn, deleteNIDs); err != nil {
				return fmt.Errorf("d.deleteQueueJSON: %w", err)
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
