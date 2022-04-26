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
	"fmt"

	userapi "github.com/matrix-org/dendrite/userapi/api"

	"github.com/matrix-org/dendrite/internal/eventutil"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/tables"
	"github.com/matrix-org/dendrite/syncapi/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/sirupsen/logrus"
)

// Database is a temporary struct until we have made syncserver.go the same for both pq/sqlite
// For now this contains the shared functions
type Database struct {
	DB                  *sql.DB
	Writer              sqlutil.Writer
	Invites             tables.Invites
	Peeks               tables.Peeks
	AccountData         tables.AccountData
	OutputEvents        tables.Events
	Topology            tables.Topology
	CurrentRoomState    tables.CurrentRoomState
	BackwardExtremities tables.BackwardsExtremities
	SendToDevice        tables.SendToDevice
	Filter              tables.Filter
	Receipts            tables.Receipts
	Memberships         tables.Memberships
	NotificationData    tables.NotificationData
	Ignores             tables.Ignores
	Presence            tables.Presence
}

func (d *Database) readOnlySnapshot(ctx context.Context) (*sql.Tx, error) {
	return d.DB.BeginTx(ctx, &sql.TxOptions{
		// Set the isolation level so that we see a snapshot of the database.
		// In PostgreSQL repeatable read transactions will see a snapshot taken
		// at the first query, and since the transaction is read-only it can't
		// run into any serialisation errors.
		// https://www.postgresql.org/docs/9.5/static/transaction-iso.html#XACT-REPEATABLE-READ
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
}

func (d *Database) MaxStreamPositionForPDUs(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.OutputEvents.SelectMaxEventID(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("d.OutputEvents.SelectMaxEventID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) MaxStreamPositionForReceipts(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.Receipts.SelectMaxReceiptID(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("d.Receipts.SelectMaxReceiptID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) MaxStreamPositionForInvites(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.Invites.SelectMaxInviteID(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("d.Invites.SelectMaxInviteID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) MaxStreamPositionForSendToDeviceMessages(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.SendToDevice.SelectMaxSendToDeviceMessageID(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("d.SendToDevice.SelectMaxSendToDeviceMessageID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) MaxStreamPositionForAccountData(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.AccountData.SelectMaxAccountDataID(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("d.Invites.SelectMaxAccountDataID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) MaxStreamPositionForNotificationData(ctx context.Context) (types.StreamPosition, error) {
	id, err := d.NotificationData.SelectMaxID(ctx)
	if err != nil {
		return 0, fmt.Errorf("d.NotificationData.SelectMaxID: %w", err)
	}
	return types.StreamPosition(id), nil
}

func (d *Database) CurrentState(ctx context.Context, roomID string, stateFilterPart *gomatrixserverlib.StateFilter, excludeEventIDs []string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectCurrentState(ctx, nil, roomID, stateFilterPart, excludeEventIDs)
}

func (d *Database) RoomIDsWithMembership(ctx context.Context, userID string, membership string) ([]string, error) {
	return d.CurrentRoomState.SelectRoomIDsWithMembership(ctx, nil, userID, membership)
}

func (d *Database) MembershipCount(ctx context.Context, roomID, membership string, pos types.StreamPosition) (int, error) {
	return d.Memberships.SelectMembershipCount(ctx, nil, roomID, membership, pos)
}

func (d *Database) RecentEvents(ctx context.Context, roomID string, r types.Range, eventFilter *gomatrixserverlib.RoomEventFilter, chronologicalOrder bool, onlySyncEvents bool) ([]types.StreamEvent, bool, error) {
	return d.OutputEvents.SelectRecentEvents(ctx, nil, roomID, r, eventFilter, chronologicalOrder, onlySyncEvents)
}

func (d *Database) PositionInTopology(ctx context.Context, eventID string) (pos types.StreamPosition, spos types.StreamPosition, err error) {
	return d.Topology.SelectPositionInTopology(ctx, nil, eventID)
}

func (d *Database) InviteEventsInRange(ctx context.Context, targetUserID string, r types.Range) (map[string]*gomatrixserverlib.HeaderedEvent, map[string]*gomatrixserverlib.HeaderedEvent, error) {
	return d.Invites.SelectInviteEventsInRange(ctx, nil, targetUserID, r)
}

func (d *Database) PeeksInRange(ctx context.Context, userID, deviceID string, r types.Range) (peeks []types.Peek, err error) {
	return d.Peeks.SelectPeeksInRange(ctx, nil, userID, deviceID, r)
}

func (d *Database) RoomReceiptsAfter(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) (types.StreamPosition, []types.OutputReceiptEvent, error) {
	return d.Receipts.SelectRoomReceiptsAfter(ctx, roomIDs, streamPos)
}

// Events lookups a list of event by their event ID.
// Returns a list of events matching the requested IDs found in the database.
// If an event is not found in the database then it will be omitted from the list.
// Returns an error if there was a problem talking with the database.
// Does not include any transaction IDs in the returned events.
func (d *Database) Events(ctx context.Context, eventIDs []string) ([]*gomatrixserverlib.HeaderedEvent, error) {
	streamEvents, err := d.OutputEvents.SelectEvents(ctx, nil, eventIDs, nil, false)
	if err != nil {
		return nil, err
	}

	// We don't include a device here as we only include transaction IDs in
	// incremental syncs.
	return d.StreamEventsToEvents(nil, streamEvents), nil
}

func (d *Database) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.CurrentRoomState.SelectJoinedUsers(ctx)
}

func (d *Database) AllPeekingDevicesInRooms(ctx context.Context) (map[string][]types.PeekingDevice, error) {
	return d.Peeks.SelectPeekingDevices(ctx)
}

func (d *Database) GetStateEvent(
	ctx context.Context, roomID, evType, stateKey string,
) (*gomatrixserverlib.HeaderedEvent, error) {
	return d.CurrentRoomState.SelectStateEvent(ctx, roomID, evType, stateKey)
}

func (d *Database) GetStateEventsForRoom(
	ctx context.Context, roomID string, stateFilter *gomatrixserverlib.StateFilter,
) (stateEvents []*gomatrixserverlib.HeaderedEvent, err error) {
	stateEvents, err = d.CurrentRoomState.SelectCurrentState(ctx, nil, roomID, stateFilter, nil)
	return
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *Database) AddInviteEvent(
	ctx context.Context, inviteEvent *gomatrixserverlib.HeaderedEvent,
) (sp types.StreamPosition, err error) {
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.Invites.InsertInviteEvent(ctx, txn, inviteEvent)
		return err
	})
	return
}

// RetireInviteEvent removes an old invite event from the database.
// Returns an error if there was a problem communicating with the database.
func (d *Database) RetireInviteEvent(
	ctx context.Context, inviteEventID string,
) (sp types.StreamPosition, err error) {
	_ = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.Invites.DeleteInviteEvent(ctx, txn, inviteEventID)
		return err
	})
	return
}

// AddPeek tracks the fact that a user has started peeking.
// If the peek was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *Database) AddPeek(
	ctx context.Context, roomID, userID, deviceID string,
) (sp types.StreamPosition, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.Peeks.InsertPeek(ctx, txn, roomID, userID, deviceID)
		return err
	})
	return
}

// DeletePeeks tracks the fact that a user has stopped peeking from the specified
// device. If the peeks was successfully deleted this returns the stream ID it was
// stored at. Returns an error if there was a problem communicating with the database.
func (d *Database) DeletePeek(
	ctx context.Context, roomID, userID, deviceID string,
) (sp types.StreamPosition, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.Peeks.DeletePeek(ctx, txn, roomID, userID, deviceID)
		return err
	})
	if err == sql.ErrNoRows {
		sp = 0
		err = nil
	}
	return
}

// DeletePeeks tracks the fact that a user has stopped peeking from all devices
// If the peeks was successfully deleted this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *Database) DeletePeeks(
	ctx context.Context, roomID, userID string,
) (sp types.StreamPosition, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.Peeks.DeletePeeks(ctx, txn, roomID, userID)
		return err
	})
	if err == sql.ErrNoRows {
		sp = 0
		err = nil
	}
	return
}

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *Database) GetAccountDataInRange(
	ctx context.Context, userID string, r types.Range,
	accountDataFilterPart *gomatrixserverlib.EventFilter,
) (map[string][]string, error) {
	return d.AccountData.SelectAccountDataInRange(ctx, userID, r, accountDataFilterPart)
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
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		sp, err = d.AccountData.InsertAccountData(ctx, txn, userID, roomID, dataType)
		return err
	})
	return
}

func (d *Database) StreamEventsToEvents(device *userapi.Device, in []types.StreamEvent) []*gomatrixserverlib.HeaderedEvent {
	out := make([]*gomatrixserverlib.HeaderedEvent, len(in))
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

// handleBackwardExtremities adds this event as a backwards extremity if and only if we do not have all of
// the events listed in the event's 'prev_events'. This function also updates the backwards extremities table
// to account for the fact that the given event is no longer a backwards extremity, but may be marked as such.
// This function should always be called within a sqlutil.Writer for safety in SQLite.
func (d *Database) handleBackwardExtremities(ctx context.Context, txn *sql.Tx, ev *gomatrixserverlib.HeaderedEvent) error {
	if err := d.BackwardExtremities.DeleteBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID()); err != nil {
		return err
	}

	// Check if we have all of the event's previous events. If an event is
	// missing, add it to the room's backward extremities.
	prevEvents, err := d.OutputEvents.SelectEvents(ctx, txn, ev.PrevEventIDs(), nil, false)
	if err != nil {
		return err
	}
	var found bool
	for _, eID := range ev.PrevEventIDs() {
		found = false
		for _, prevEv := range prevEvents {
			if eID == prevEv.EventID() {
				found = true
			}
		}

		// If the event is missing, consider it a backward extremity.
		if !found {
			if err = d.BackwardExtremities.InsertsBackwardExtremity(ctx, txn, ev.RoomID(), ev.EventID(), eID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *Database) PurgeRoomState(
	ctx context.Context, roomID string,
) error {
	return d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		// If the event is a create event then we'll delete all of the existing
		// data for the room. The only reason that a create event would be replayed
		// to us in this way is if we're about to receive the entire room state.
		if err := d.CurrentRoomState.DeleteRoomStateForRoom(ctx, txn, roomID); err != nil {
			return fmt.Errorf("d.CurrentRoomState.DeleteRoomStateForRoom: %w", err)
		}
		return nil
	})
}

func (d *Database) WriteEvent(
	ctx context.Context,
	ev *gomatrixserverlib.HeaderedEvent,
	addStateEvents []*gomatrixserverlib.HeaderedEvent,
	addStateEventIDs, removeStateEventIDs []string,
	transactionID *api.TransactionID, excludeFromSync bool,
) (pduPosition types.StreamPosition, returnErr error) {
	returnErr = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		var err error
		pos, err := d.OutputEvents.InsertEvent(
			ctx, txn, ev, addStateEventIDs, removeStateEventIDs, transactionID, excludeFromSync,
		)
		if err != nil {
			return fmt.Errorf("d.OutputEvents.InsertEvent: %w", err)
		}
		pduPosition = pos
		var topoPosition types.StreamPosition
		if topoPosition, err = d.Topology.InsertEventInTopology(ctx, txn, ev, pos); err != nil {
			return fmt.Errorf("d.Topology.InsertEventInTopology: %w", err)
		}

		if err = d.handleBackwardExtremities(ctx, txn, ev); err != nil {
			return fmt.Errorf("d.handleBackwardExtremities: %w", err)
		}

		if len(addStateEvents) == 0 && len(removeStateEventIDs) == 0 {
			// Nothing to do, the event may have just been a message event.
			return nil
		}

		return d.updateRoomState(ctx, txn, removeStateEventIDs, addStateEvents, pduPosition, topoPosition)
	})

	return pduPosition, returnErr
}

// This function should always be called within a sqlutil.Writer for safety in SQLite.
func (d *Database) updateRoomState(
	ctx context.Context, txn *sql.Tx,
	removedEventIDs []string,
	addedEvents []*gomatrixserverlib.HeaderedEvent,
	pduPosition types.StreamPosition,
	topoPosition types.StreamPosition,
) error {
	// remove first, then add, as we do not ever delete state, but do replace state which is a remove followed by an add.
	for _, eventID := range removedEventIDs {
		if err := d.CurrentRoomState.DeleteRoomStateByEventID(ctx, txn, eventID); err != nil {
			return fmt.Errorf("d.CurrentRoomState.DeleteRoomStateByEventID: %w", err)
		}
	}

	for _, event := range addedEvents {
		if event.StateKey() == nil {
			// ignore non state events
			continue
		}
		var membership *string
		if event.Type() == "m.room.member" {
			value, err := event.Membership()
			if err != nil {
				return fmt.Errorf("event.Membership: %w", err)
			}
			membership = &value
			if err = d.Memberships.UpsertMembership(ctx, txn, event, pduPosition, topoPosition); err != nil {
				return fmt.Errorf("d.Memberships.UpsertMembership: %w", err)
			}
		}

		if err := d.CurrentRoomState.UpsertRoomState(ctx, txn, event, membership, pduPosition); err != nil {
			return fmt.Errorf("d.CurrentRoomState.UpsertRoomState: %w", err)
		}
	}

	return nil
}

func (d *Database) GetEventsInTopologicalRange(
	ctx context.Context,
	from, to *types.TopologyToken,
	roomID string,
	filter *gomatrixserverlib.RoomEventFilter,
	backwardOrdering bool,
) (events []types.StreamEvent, err error) {
	var minDepth, maxDepth, maxStreamPosForMaxDepth types.StreamPosition
	if backwardOrdering {
		// Backward ordering means the 'from' token has a higher depth than the 'to' token
		minDepth = to.Depth
		maxDepth = from.Depth
		// for cases where we have say 5 events with the same depth, the TopologyToken needs to
		// know which of the 5 the client has seen. This is done by using the PDU position.
		// Events with the same maxDepth but less than this PDU position will be returned.
		maxStreamPosForMaxDepth = from.PDUPosition
	} else {
		// Forward ordering means the 'from' token has a lower depth than the 'to' token.
		minDepth = from.Depth
		maxDepth = to.Depth
	}

	// Select the event IDs from the defined range.
	var eIDs []string
	eIDs, err = d.Topology.SelectEventIDsInRange(
		ctx, nil, roomID, minDepth, maxDepth, maxStreamPosForMaxDepth, filter.Limit, !backwardOrdering,
	)
	if err != nil {
		return
	}

	// Retrieve the events' contents using their IDs.
	events, err = d.OutputEvents.SelectEvents(ctx, nil, eIDs, filter, true)
	return
}

func (d *Database) BackwardExtremitiesForRoom(
	ctx context.Context, roomID string,
) (backwardExtremities map[string][]string, err error) {
	return d.BackwardExtremities.SelectBackwardExtremitiesForRoom(ctx, roomID)
}

func (d *Database) MaxTopologicalPosition(
	ctx context.Context, roomID string,
) (types.TopologyToken, error) {
	depth, streamPos, err := d.Topology.SelectMaxPositionInTopology(ctx, nil, roomID)
	if err != nil {
		return types.TopologyToken{}, err
	}
	return types.TopologyToken{Depth: depth, PDUPosition: streamPos}, nil
}

func (d *Database) EventPositionInTopology(
	ctx context.Context, eventID string,
) (types.TopologyToken, error) {
	depth, stream, err := d.Topology.SelectPositionInTopology(ctx, nil, eventID)
	if err != nil {
		return types.TopologyToken{}, err
	}
	return types.TopologyToken{Depth: depth, PDUPosition: stream}, nil
}

func (d *Database) StreamToTopologicalPosition(
	ctx context.Context, roomID string, streamPos types.StreamPosition, backwardOrdering bool,
) (types.TopologyToken, error) {
	topoPos, err := d.Topology.SelectStreamToTopologicalPosition(ctx, nil, roomID, streamPos, backwardOrdering)
	switch {
	case err == sql.ErrNoRows && backwardOrdering: // no events in range, going backward
		return types.TopologyToken{PDUPosition: streamPos}, nil
	case err == sql.ErrNoRows && !backwardOrdering: // no events in range, going forward
		topoPos, streamPos, err = d.Topology.SelectMaxPositionInTopology(ctx, nil, roomID)
		if err != nil {
			return types.TopologyToken{}, fmt.Errorf("d.Topology.SelectMaxPositionInTopology: %w", err)
		}
		return types.TopologyToken{Depth: topoPos, PDUPosition: streamPos}, nil
	case err != nil: // some other error happened
		return types.TopologyToken{}, fmt.Errorf("d.Topology.SelectStreamToTopologicalPosition: %w", err)
	default:
		return types.TopologyToken{Depth: topoPos, PDUPosition: streamPos}, nil
	}
}

func (d *Database) GetFilter(
	ctx context.Context, localpart string, filterID string,
) (*gomatrixserverlib.Filter, error) {
	return d.Filter.SelectFilter(ctx, localpart, filterID)
}

func (d *Database) PutFilter(
	ctx context.Context, localpart string, filter *gomatrixserverlib.Filter,
) (string, error) {
	var filterID string
	var err error
	err = d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		filterID, err = d.Filter.InsertFilter(ctx, filter, localpart)
		return err
	})
	return filterID, err
}

func (d *Database) RedactEvent(ctx context.Context, redactedEventID string, redactedBecause *gomatrixserverlib.HeaderedEvent) error {
	redactedEvents, err := d.Events(ctx, []string{redactedEventID})
	if err != nil {
		return err
	}
	if len(redactedEvents) == 0 {
		logrus.WithField("event_id", redactedEventID).WithField("redaction_event", redactedBecause.EventID()).Warnf("missing redacted event for redaction")
		return nil
	}
	eventToRedact := redactedEvents[0].Unwrap()
	redactionEvent := redactedBecause.Unwrap()
	ev, err := eventutil.RedactEvent(redactionEvent, eventToRedact)
	if err != nil {
		return err
	}

	newEvent := ev.Headered(redactedBecause.RoomVersion)
	err = d.Writer.Do(nil, nil, func(txn *sql.Tx) error {
		return d.OutputEvents.UpdateEventJSON(ctx, newEvent)
	})
	return err
}

// Retrieve the backward topology position, i.e. the position of the
// oldest event in the room's topology.
func (d *Database) GetBackwardTopologyPos(
	ctx context.Context,
	events []types.StreamEvent,
) (types.TopologyToken, error) {
	zeroToken := types.TopologyToken{}
	if len(events) == 0 {
		return zeroToken, nil
	}
	pos, spos, err := d.Topology.SelectPositionInTopology(ctx, nil, events[0].EventID())
	if err != nil {
		return zeroToken, err
	}
	tok := types.TopologyToken{Depth: pos, PDUPosition: spos}
	tok.Decrement()
	return tok, nil
}

// fetchStateEvents converts the set of event IDs into a set of events. It will fetch any which are missing from the database.
// Returns a map of room ID to list of events.
func (d *Database) fetchStateEvents(
	ctx context.Context, txn *sql.Tx,
	roomIDToEventIDSet map[string]map[string]bool,
	eventIDToEvent map[string]types.StreamEvent,
) (map[string][]types.StreamEvent, error) {
	stateBetween := make(map[string][]types.StreamEvent)
	missingEvents := make(map[string][]string)
	for roomID, ids := range roomIDToEventIDSet {
		events := stateBetween[roomID]
		for id, need := range ids {
			if !need {
				continue // deleted state
			}
			e, ok := eventIDToEvent[id]
			if ok {
				events = append(events, e)
			} else {
				m := missingEvents[roomID]
				m = append(m, id)
				missingEvents[roomID] = m
			}
		}
		stateBetween[roomID] = events
	}

	if len(missingEvents) > 0 {
		// This happens when add_state_ids has an event ID which is not in the provided range.
		// We need to explicitly fetch them.
		allMissingEventIDs := []string{}
		for _, missingEvIDs := range missingEvents {
			allMissingEventIDs = append(allMissingEventIDs, missingEvIDs...)
		}
		evs, err := d.fetchMissingStateEvents(ctx, txn, allMissingEventIDs)
		if err != nil {
			return nil, err
		}
		// we know we got them all otherwise an error would've been returned, so just loop the events
		for _, ev := range evs {
			roomID := ev.RoomID()
			stateBetween[roomID] = append(stateBetween[roomID], ev)
		}
	}
	return stateBetween, nil
}

func (d *Database) fetchMissingStateEvents(
	ctx context.Context, txn *sql.Tx, eventIDs []string,
) ([]types.StreamEvent, error) {
	// Fetch from the events table first so we pick up the stream ID for the
	// event.
	events, err := d.OutputEvents.SelectEvents(ctx, txn, eventIDs, nil, false)
	if err != nil {
		return nil, err
	}

	have := map[string]bool{}
	for _, event := range events {
		have[event.EventID()] = true
	}
	var missing []string
	for _, eventID := range eventIDs {
		if !have[eventID] {
			missing = append(missing, eventID)
		}
	}
	if len(missing) == 0 {
		return events, nil
	}

	// If they are missing from the events table then they should be state
	// events that we received from outside the main event stream.
	// These should be in the room state table.
	stateEvents, err := d.CurrentRoomState.SelectEventsWithEventIDs(ctx, txn, missing)

	if err != nil {
		return nil, err
	}
	if len(stateEvents) != len(missing) {
		logrus.WithContext(ctx).Warnf("Failed to map all event IDs to events (got %d, wanted %d)", len(stateEvents), len(missing))

		// TODO: Why is this happening? It's probably the roomserver. Uncomment
		// this error again when we work out what it is and fix it, otherwise we
		// just end up returning lots of 500s to the client and that breaks
		// pretty much everything, rather than just sending what we have.
		//return nil, fmt.Errorf("failed to map all event IDs to events: (got %d, wanted %d)", len(stateEvents), len(missing))
	}
	events = append(events, stateEvents...)
	return events, nil
}

// getStateDeltas returns the state deltas between fromPos and toPos,
// exclusive of oldPos, inclusive of newPos, for the rooms in which
// the user has new membership events.
// A list of joined room IDs is also returned in case the caller needs it.
func (d *Database) GetStateDeltas(
	ctx context.Context, device *userapi.Device,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StateDelta, []string, error) {
	// Implement membership change algorithm: https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821
	// - Get membership list changes for this user in this sync response
	// - For each room which has membership list changes:
	//     * Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins TODO).
	//       If it is, then we need to send the full room state down (and 'limited' is always true).
	//     * Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
	//     * Check if the user is CURRENTLY (TODO) left/banned. If so, add room to 'archived' block.
	// - Get all CURRENTLY joined rooms, and add them to 'joined' block.
	txn, err := d.readOnlySnapshot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("d.readOnlySnapshot: %w", err)
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(txn, &succeeded, &err)

	// Look up all memberships for the user. We only care about rooms that a
	// user has ever interacted with — joined to, kicked/banned from, left.
	memberships, err := d.CurrentRoomState.SelectRoomIDsWithAnyMembership(ctx, txn, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	allRoomIDs := make([]string, 0, len(memberships))
	joinedRoomIDs := make([]string, 0, len(memberships))
	for roomID, membership := range memberships {
		allRoomIDs = append(allRoomIDs, roomID)
		if membership == gomatrixserverlib.Join {
			joinedRoomIDs = append(joinedRoomIDs, roomID)
		}
	}

	var deltas []types.StateDelta

	// get all the state events ever (i.e. for all available rooms) between these two positions
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, txn, r, stateFilter, allRoomIDs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// find out which rooms this user is peeking, if any.
	// We do this before joins so any peeks get overwritten
	peeks, err := d.Peeks.SelectPeeksInRange(ctx, txn, userID, device.ID, r)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, err
	}

	// add peek blocks
	for _, peek := range peeks {
		if peek.New {
			// send full room state down instead of a delta
			var s []types.StreamEvent
			s, err = d.currentStateStreamEventsForRoom(ctx, txn, peek.RoomID, stateFilter)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				return nil, nil, err
			}
			state[peek.RoomID] = s
		}
		if !peek.Deleted {
			deltas = append(deltas, types.StateDelta{
				Membership:  gomatrixserverlib.Peek,
				StateEvents: d.StreamEventsToEvents(device, state[peek.RoomID]),
				RoomID:      peek.RoomID,
			})
		}
	}

	// handle newly joined rooms and non-joined rooms
	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			// TODO: Currently this will incorrectly add rooms which were ALREADY joined but they sent another no-op join event.
			//       We should be checking if the user was already joined at fromPos and not proceed if so. As a result of this,
			//       dupe join events will result in the entire room state coming down to the client again. This is added in
			//       the 'state' part of the response though, so is transparent modulo bandwidth concerns as it is not added to
			//       the timeline.
			if membership := getMembershipFromEvent(ev.Event, userID); membership != "" {
				if membership == gomatrixserverlib.Join {
					// send full room state down instead of a delta
					var s []types.StreamEvent
					s, err = d.currentStateStreamEventsForRoom(ctx, txn, roomID, stateFilter)
					if err != nil {
						if err == sql.ErrNoRows {
							continue
						}
						return nil, nil, err
					}
					state[roomID] = s
					continue // we'll add this room in when we do joined rooms
				}

				deltas = append(deltas, types.StateDelta{
					Membership:    membership,
					MembershipPos: ev.StreamPosition,
					StateEvents:   d.StreamEventsToEvents(device, stateStreamEvents),
					RoomID:        roomID,
				})
				break
			}
		}
	}

	// Add in currently joined rooms
	for _, joinedRoomID := range joinedRoomIDs {
		deltas = append(deltas, types.StateDelta{
			Membership:  gomatrixserverlib.Join,
			StateEvents: d.StreamEventsToEvents(device, state[joinedRoomID]),
			RoomID:      joinedRoomID,
		})
	}

	succeeded = true
	return deltas, joinedRoomIDs, nil
}

// getStateDeltasForFullStateSync is a variant of getStateDeltas used for /sync
// requests with full_state=true.
// Fetches full state for all joined rooms and uses selectStateInRange to get
// updates for other rooms.
func (d *Database) GetStateDeltasForFullStateSync(
	ctx context.Context, device *userapi.Device,
	r types.Range, userID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StateDelta, []string, error) {
	txn, err := d.readOnlySnapshot(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("d.readOnlySnapshot: %w", err)
	}
	var succeeded bool
	defer sqlutil.EndTransactionWithCheck(txn, &succeeded, &err)

	// Look up all memberships for the user. We only care about rooms that a
	// user has ever interacted with — joined to, kicked/banned from, left.
	memberships, err := d.CurrentRoomState.SelectRoomIDsWithAnyMembership(ctx, txn, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	allRoomIDs := make([]string, 0, len(memberships))
	joinedRoomIDs := make([]string, 0, len(memberships))
	for roomID, membership := range memberships {
		allRoomIDs = append(allRoomIDs, roomID)
		if membership == gomatrixserverlib.Join {
			joinedRoomIDs = append(joinedRoomIDs, roomID)
		}
	}

	// Use a reasonable initial capacity
	deltas := make(map[string]types.StateDelta)

	peeks, err := d.Peeks.SelectPeeksInRange(ctx, txn, userID, device.ID, r)
	if err != nil && err != sql.ErrNoRows {
		return nil, nil, err
	}

	// Add full states for all peeking rooms
	for _, peek := range peeks {
		if !peek.Deleted {
			s, stateErr := d.currentStateStreamEventsForRoom(ctx, txn, peek.RoomID, stateFilter)
			if stateErr != nil {
				if stateErr == sql.ErrNoRows {
					continue
				}
				return nil, nil, stateErr
			}
			deltas[peek.RoomID] = types.StateDelta{
				Membership:  gomatrixserverlib.Peek,
				StateEvents: d.StreamEventsToEvents(device, s),
				RoomID:      peek.RoomID,
			}
		}
	}

	// Get all the state events ever between these two positions
	stateNeeded, eventMap, err := d.OutputEvents.SelectStateInRange(ctx, txn, r, stateFilter, allRoomIDs)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	state, err := d.fetchStateEvents(ctx, txn, stateNeeded, eventMap)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	for roomID, stateStreamEvents := range state {
		for _, ev := range stateStreamEvents {
			if membership := getMembershipFromEvent(ev.Event, userID); membership != "" {
				if membership != gomatrixserverlib.Join { // We've already added full state for all joined rooms above.
					deltas[roomID] = types.StateDelta{
						Membership:    membership,
						MembershipPos: ev.StreamPosition,
						StateEvents:   d.StreamEventsToEvents(device, stateStreamEvents),
						RoomID:        roomID,
					}
				}

				break
			}
		}
	}

	// Add full states for all joined rooms
	for _, joinedRoomID := range joinedRoomIDs {
		s, stateErr := d.currentStateStreamEventsForRoom(ctx, txn, joinedRoomID, stateFilter)
		if stateErr != nil {
			if stateErr == sql.ErrNoRows {
				continue
			}
			return nil, nil, stateErr
		}
		deltas[joinedRoomID] = types.StateDelta{
			Membership:  gomatrixserverlib.Join,
			StateEvents: d.StreamEventsToEvents(device, s),
			RoomID:      joinedRoomID,
		}
	}

	// Create a response array.
	result := make([]types.StateDelta, len(deltas))
	i := 0
	for _, delta := range deltas {
		result[i] = delta
		i++
	}

	succeeded = true
	return result, joinedRoomIDs, nil
}

func (d *Database) currentStateStreamEventsForRoom(
	ctx context.Context, txn *sql.Tx, roomID string,
	stateFilter *gomatrixserverlib.StateFilter,
) ([]types.StreamEvent, error) {
	allState, err := d.CurrentRoomState.SelectCurrentState(ctx, txn, roomID, stateFilter, nil)
	if err != nil {
		return nil, err
	}
	s := make([]types.StreamEvent, len(allState))
	for i := 0; i < len(s); i++ {
		s[i] = types.StreamEvent{HeaderedEvent: allState[i], StreamPosition: 0}
	}
	return s, nil
}

func (d *Database) StoreNewSendForDeviceMessage(
	ctx context.Context, userID, deviceID string, event gomatrixserverlib.SendToDeviceEvent,
) (newPos types.StreamPosition, err error) {
	j, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}
	// Delegate the database write task to the SendToDeviceWriter. It'll guarantee
	// that we don't lock the table for writes in more than one place.
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		newPos, err = d.SendToDevice.InsertSendToDeviceMessage(
			ctx, txn, userID, deviceID, string(j),
		)
		return err
	})
	if err != nil {
		return 0, err
	}
	return newPos, nil
}

func (d *Database) SendToDeviceUpdatesForSync(
	ctx context.Context,
	userID, deviceID string,
	from, to types.StreamPosition,
) (types.StreamPosition, []types.SendToDeviceEvent, error) {
	// First of all, get our send-to-device updates for this user.
	lastPos, events, err := d.SendToDevice.SelectSendToDeviceMessages(ctx, nil, userID, deviceID, from, to)
	if err != nil {
		return from, nil, fmt.Errorf("d.SendToDevice.SelectSendToDeviceMessages: %w", err)
	}
	// If there's nothing to do then stop here.
	if len(events) == 0 {
		return to, nil, nil
	}
	return lastPos, events, nil
}

func (d *Database) CleanSendToDeviceUpdates(
	ctx context.Context,
	userID, deviceID string, before types.StreamPosition,
) (err error) {
	if err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		return d.SendToDevice.DeleteSendToDeviceMessages(ctx, txn, userID, deviceID, before)
	}); err != nil {
		logrus.WithError(err).Errorf("Failed to clean up old send-to-device messages for user %q device %q", userID, deviceID)
		return err
	}
	return nil
}

// getMembershipFromEvent returns the value of content.membership iff the event is a state event
// with type 'm.room.member' and state_key of userID. Otherwise, an empty string is returned.
func getMembershipFromEvent(ev *gomatrixserverlib.Event, userID string) string {
	if ev.Type() != "m.room.member" || !ev.StateKeyEquals(userID) {
		return ""
	}
	membership, err := ev.Membership()
	if err != nil {
		return ""
	}
	return membership
}

// StoreReceipt stores user receipts
func (d *Database) StoreReceipt(ctx context.Context, roomId, receiptType, userId, eventId string, timestamp gomatrixserverlib.Timestamp) (pos types.StreamPosition, err error) {
	err = d.Writer.Do(d.DB, nil, func(txn *sql.Tx) error {
		pos, err = d.Receipts.UpsertReceipt(ctx, txn, roomId, receiptType, userId, eventId, timestamp)
		return err
	})
	return
}

func (d *Database) GetRoomReceipts(ctx context.Context, roomIDs []string, streamPos types.StreamPosition) ([]types.OutputReceiptEvent, error) {
	_, receipts, err := d.Receipts.SelectRoomReceiptsAfter(ctx, roomIDs, streamPos)
	return receipts, err
}

func (d *Database) UpsertRoomUnreadNotificationCounts(ctx context.Context, userID, roomID string, notificationCount, highlightCount int) (pos types.StreamPosition, err error) {
	err = d.Writer.Do(nil, nil, func(_ *sql.Tx) error {
		pos, err = d.NotificationData.UpsertRoomUnreadCounts(ctx, userID, roomID, notificationCount, highlightCount)
		return err
	})
	return
}

func (d *Database) GetUserUnreadNotificationCounts(ctx context.Context, userID string, from, to types.StreamPosition) (map[string]*eventutil.NotificationData, error) {
	return d.NotificationData.SelectUserUnreadCounts(ctx, userID, from, to)
}

func (s *Database) SelectContextEvent(ctx context.Context, roomID, eventID string) (int, gomatrixserverlib.HeaderedEvent, error) {
	return s.OutputEvents.SelectContextEvent(ctx, nil, roomID, eventID)
}

func (s *Database) SelectContextBeforeEvent(ctx context.Context, id int, roomID string, filter *gomatrixserverlib.RoomEventFilter) ([]*gomatrixserverlib.HeaderedEvent, error) {
	return s.OutputEvents.SelectContextBeforeEvent(ctx, nil, id, roomID, filter)
}
func (s *Database) SelectContextAfterEvent(ctx context.Context, id int, roomID string, filter *gomatrixserverlib.RoomEventFilter) (int, []*gomatrixserverlib.HeaderedEvent, error) {
	return s.OutputEvents.SelectContextAfterEvent(ctx, nil, id, roomID, filter)
}

func (s *Database) IgnoresForUser(ctx context.Context, userID string) (*types.IgnoredUsers, error) {
	return s.Ignores.SelectIgnores(ctx, userID)
}

func (s *Database) UpdateIgnoresForUser(ctx context.Context, userID string, ignores *types.IgnoredUsers) error {
	return s.Ignores.UpsertIgnores(ctx, userID, ignores)
}

func (s *Database) UpdatePresence(ctx context.Context, userID string, presence types.Presence, statusMsg *string, lastActiveTS gomatrixserverlib.Timestamp, fromSync bool) (types.StreamPosition, error) {
	return s.Presence.UpsertPresence(ctx, nil, userID, statusMsg, presence, lastActiveTS, fromSync)
}

func (s *Database) GetPresence(ctx context.Context, userID string) (*types.PresenceInternal, error) {
	return s.Presence.GetPresenceForUser(ctx, nil, userID)
}

func (s *Database) PresenceAfter(ctx context.Context, after types.StreamPosition) (map[string]*types.PresenceInternal, error) {
	return s.Presence.GetPresenceAfter(ctx, nil, after)
}

func (s *Database) MaxStreamPositionForPresence(ctx context.Context) (types.StreamPosition, error) {
	return s.Presence.GetMaxPresenceID(ctx, nil)
}
