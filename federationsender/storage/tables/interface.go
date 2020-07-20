package tables

import (
	"context"
	"database/sql"

	"github.com/matrix-org/dendrite/federationsender/types"
	"github.com/matrix-org/gomatrixserverlib"
)

type FederationSenderQueuePDUs interface {
	InsertQueuePDU(ctx context.Context, txn *sql.Tx, transactionID gomatrixserverlib.TransactionID, serverName gomatrixserverlib.ServerName, nid int64) error
	DeleteQueuePDUTransaction(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, transactionID gomatrixserverlib.TransactionID) error
	SelectQueuePDUNextTransactionID(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (gomatrixserverlib.TransactionID, error)
	SelectQueuePDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID int64) (int64, error)
	SelectQueuePDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
	SelectQueuePDUs(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName, transactionID gomatrixserverlib.TransactionID, limit int) ([]int64, error)
	SelectQueuePDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderQueueEDUs interface {
	InsertQueueEDU(ctx context.Context, txn *sql.Tx, userID, deviceID string, serverName gomatrixserverlib.ServerName, nid int64) error
	SelectQueueEDU(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) ([]int64, error)
	SelectQueueEDUReferenceJSONCount(ctx context.Context, txn *sql.Tx, jsonNID int64) (int64, error)
	SelectQueueEDUCount(ctx context.Context, txn *sql.Tx, serverName gomatrixserverlib.ServerName) (int64, error)
	SelectQueueEDUServerNames(ctx context.Context, txn *sql.Tx) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderQueueJSON interface {
	InsertQueueJSON(ctx context.Context, txn *sql.Tx, json string) (int64, error)
	DeleteQueueJSON(ctx context.Context, txn *sql.Tx, nids []int64) error
	SelectQueueJSON(ctx context.Context, txn *sql.Tx, jsonNIDs []int64) (map[int64][]byte, error)
}

type FederationSenderJoinedHosts interface {
	InsertJoinedHosts(ctx context.Context, txn *sql.Tx, roomID, eventID string, serverName gomatrixserverlib.ServerName) error
	DeleteJoinedHosts(ctx context.Context, txn *sql.Tx, eventIDs []string) error
	SelectJoinedHostsWithTx(ctx context.Context, txn *sql.Tx, roomID string) ([]types.JoinedHost, error)
	SelectJoinedHosts(ctx context.Context, roomID string) ([]types.JoinedHost, error)
	SelectAllJoinedHosts(ctx context.Context) ([]gomatrixserverlib.ServerName, error)
}

type FederationSenderRooms interface {
	InsertRoom(ctx context.Context, txn *sql.Tx, roomID string) error
	SelectRoomForUpdate(ctx context.Context, txn *sql.Tx, roomID string) (string, error)
	UpdateRoom(ctx context.Context, txn *sql.Tx, roomID, lastEventID string) error
}
