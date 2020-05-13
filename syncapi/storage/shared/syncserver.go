package shared

import (
	"context"
	"database/sql"
	"time"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// Database is a temporary struct until we have made syncserver.go the same for both pq/sqlite
// For now this contains the shared functions
type Database struct {
	DB           *sql.DB
	Invites      tables.Invites
	AccountData  tables.AccountData
	OutputEvents tables.Events
	EDUCache     *cache.EDUCache
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *Database) Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error) {
	streamEvents, err := d.OutputEvents.SelectEvents(ctx, nil, eventIDs)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return d.StreamEventsToEvents(nil, streamEvents), nil
}

// GetEventsInStreamingRange retrieves all of the events on a given ordering using the
// given extremities and limit.
func (d *Database) GetEventsInStreamingRange(
	ctx context.Context,
	from, to *types.StreamingToken,
	roomID string, limit int,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	if backwardOrdering {
		// When using backward ordering, we want the most recent events first.
		if events, err = d.OutputEvents.SelectRecentEvents(
			ctx, nil, roomID, to.PDUPosition(), from.PDUPosition(), limit, false, false,
		); err != nil {
			return
		}
	} else {
		// When using forward ordering, we want the least recent events first.
		if events, err = d.OutputEvents.SelectEarlyEvents(
			ctx, nil, roomID, from.PDUPosition(), to.PDUPosition(), limit,
		); err != nil {
			return
		}
	}
	return events, err
}

func (d *Database) AddTypingUser(
	userID, roomID string, expireTime *time.Time,
) types.StreamPosition {
	return types.StreamPosition(d.EDUCache.AddTypingUser(userID, roomID, expireTime))
}

func (d *Database) RemoveTypingUser(
	userID, roomID string,
) types.StreamPosition {
	return types.StreamPosition(d.EDUCache.RemoveUser(userID, roomID))
}

func (d *Database) SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn) {
	d.EDUCache.SetTimeoutCallback(fn)
}

func (d *Database) SyncStreamPosition(ctx context.Context) (types.StreamPosition, error) {
	var maxID int64
	var err error
	err = common.WithTransaction(d.DB, func(txn *sql.Tx) error {
		maxID, err = d.OutputEvents.SelectMaxEventID(ctx, txn)
		if err != nil {
			return err
		}
		maxAccountDataID, err := d.AccountData.SelectMaxAccountDataID(ctx, txn)
		if err != nil {
			return err
		}
		if maxAccountDataID > maxID {
			maxID = maxAccountDataID
		}
		maxInviteID, err := d.Invites.SelectMaxInviteID(ctx, txn)
		if err != nil {
			return err
		}
		if maxInviteID > maxID {
			maxID = maxInviteID
		}
		return nil
	})
	return types.StreamPosition(maxID), err
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *Database) AddInviteEvent(
	ctx context.Context, inviteEvent gomatrixserverlib.HeaderedEvent,
) (sp types.StreamPosition, err error) {
	err = common.WithTransaction(d.DB, func(txn *sql.Tx) error {
		sp, err = d.Invites.InsertInviteEvent(ctx, txn, inviteEvent)
		return err
	})
	return
}

// RetireInviteEvent removes an old invite event from the database.
// Returns an error if there was a problem communicating with the database.
func (d *Database) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) error {
	// TODO: Record that invite has been retired in a stream so that we can
	// notify the user in an incremental sync.
	err := d.Invites.DeleteInviteEvent(ctx, inviteEventID)
	return err
}

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *Database) GetAccountDataInRange(
	ctx context.Context, userID string, oldPos, newPos types.StreamPosition,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (map[string][]string, error) {
	return d.AccountData.SelectAccountDataInRange(ctx, userID, oldPos, newPos, accountDataFilterPart)
}

// UpsertAccountData keeps track of new or updated account data, by saving the type
// of the new/updated data, and the user ID and room ID the data is related to (empty)
// room ID means the data isn't specific to any room)
// If no data with the given type, user ID and room ID exists in the database,
// creates a new row, else update the existing one
// Returns an error if there was an issue with the upsert
func (d *Database) UpsertAccountData(
	ctx context.Context, userID, roomID, dataType string,
) (sp types.StreamPosition, err error) {
	err = common.WithTransaction(d.DB, func(txn *sql.Tx) error {
		sp, err = d.AccountData.InsertAccountData(ctx, txn, userID, roomID, dataType)
		return err
	})
	return
}

func (d *Database) StreamEventsToEvents(device *authtypes.Device, in []types.StreamEvent) []gomatrixserverlib.HeaderedEvent {
	out := make([]gomatrixserverlib.HeaderedEvent, len(in))
	for i := 0; i < len(in); i++ {
		out[i] = in[i].HeaderedEvent
		if device != nil && in[i].TransactionID != nil {
			if device.UserID == in[i].Sender() && device.SessionID == in[i].TransactionID.SessionID {
				err := out[i].SetUnsignedField(
					"transaction_id", in[i].TransactionID.TransactionID,
				)
				if err != nil {
					logrus.WithFields(logrus.Fields{
						"event_id": out[i].EventID(),
					}).WithError(err).Warnf("Failed to add transaction ID to event")
				}
			}
		}
	}
	return out
}
