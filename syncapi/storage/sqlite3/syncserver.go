// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"context"
	"database/sql"

	// Import the sqlite3 package
	_ "github.com/mattn/go-sqlite3"

	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/sqlutil"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/storage/shared"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

// SyncServerDatasource represents a sync server datasource which manages
// both the database for PDUs and caches for EDUs.
type SyncServerDatasource struct {
	shared.Database
	db     *sql.DB
	writer sqlutil.Writer
	sqlutil.PartitionOffsetStatements
	streamID streamIDStatements
	dbctx    context.Context
}

// NewDatabase creates a new sync server database
// nolint: gocyclo
func NewDatabase(dbProperties *config.DatabaseOptions) (*SyncServerDatasource, error) {
	var d SyncServerDatasource
	var err error
	d.writer = sqlutil.NewExclusiveWriter()
	d.db, d.dbctx, err = sqlutil.OpenWithWriter(dbProperties, d.writer)
	if err != nil {
		return nil, err
	}

	if err = d.prepare(); err != nil {
		return nil, err
	}
	return &d, nil
}

func (d *SyncServerDatasource) prepare() (err error) {
	if err = d.PartitionOffsetStatements.Prepare(d.db, d.writer, "syncapi"); err != nil {
		return err
	}
	if err = d.streamID.prepare(d.db); err != nil {
		return err
	}
	accountData, err := NewSqliteAccountDataTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	events, err := NewSqliteEventsTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	roomState, err := NewSqliteCurrentRoomStateTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	invites, err := NewSqliteInvitesTable(d.db, &d.streamID)
	if err != nil {
		return err
	}
	topology, err := NewSqliteTopologyTable(d.db)
	if err != nil {
		return err
	}
	bwExtrem, err := NewSqliteBackwardsExtremitiesTable(d.db)
	if err != nil {
		return err
	}
	sendToDevice, err := NewSqliteSendToDeviceTable(d.db)
	if err != nil {
		return err
	}
	filter, err := NewSqliteFilterTable(d.db)
	if err != nil {
		return err
	}
	d.Database = shared.Database{
		DB:                  d.db,
		Writer:              d.writer,
		Invites:             invites,
		AccountData:         accountData,
		OutputEvents:        events,
		BackwardExtremities: bwExtrem,
		CurrentRoomState:    roomState,
		Topology:            topology,
		Filter:              filter,
		SendToDevice:        sendToDevice,
		EDUCache:            cache.New(),
	}
	return nil
}

func (d *SyncServerDatasource) Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error) {
	return d.Database.Events(d.dbctx, eventIDs)
}
func (d *SyncServerDatasource) WriteEvent(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent, addStateEvents []gomatrixserverlib.HeaderedEvent,
	addStateEventIDs []string, removeStateEventIDs []string, transactionID *api.TransactionID, excludeFromSync bool) (types.StreamPosition, error) {
	return d.Database.WriteEvent(d.dbctx, ev, addStateEvents, addStateEventIDs, removeStateEventIDs, transactionID, excludeFromSync)
}
func (d *SyncServerDatasource) AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error) {
	return d.Database.AllJoinedUsersInRooms(d.dbctx)
}

func (d *SyncServerDatasource) GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error) {
	return d.Database.GetStateEvent(d.dbctx, roomID, evType, stateKey)
}

// GetStateEventsForRoom fetches the state events for a given room.
// Returns an empty slice if no state events could be found for this room.
// Returns an error if there was an issue with the retrieval.
func (d *SyncServerDatasource) GetStateEventsForRoom(ctx context.Context, roomID string, stateFilterPart *gomatrixserverlib.StateFilter) (stateEvents []gomatrixserverlib.HeaderedEvent, err error) {
	return d.Database.GetStateEventsForRoom(d.dbctx, roomID, stateFilterPart)
}

// SyncPosition returns the latest positions for syncing.
func (d *SyncServerDatasource) SyncPosition(ctx context.Context) (types.StreamingToken, error) {
	return d.Database.SyncPosition(d.dbctx)
}

func (d *SyncServerDatasource) IncrementalSync(ctx context.Context, res *types.Response, device userapi.Device, fromPos, toPos types.StreamingToken, numRecentEventsPerRoom int, wantFullState bool) (*types.Response, error) {
	return d.Database.IncrementalSync(d.dbctx, res, device, fromPos, toPos, numRecentEventsPerRoom, wantFullState)
}

// CompleteSync returns a complete /sync API response for the given user. A response object
// must be provided for CompleteSync to populate - it will not create one.
func (d *SyncServerDatasource) CompleteSync(ctx context.Context, res *types.Response, device userapi.Device, numRecentEventsPerRoom int) (*types.Response, error) {
	return d.Database.CompleteSync(d.dbctx, res, device, numRecentEventsPerRoom)
}

// GetAccountDataInRange returns all account data for a given user inserted or
// updated between two given positions
// Returns a map following the format data[roomID] = []dataTypes
// If no data is retrieved, returns an empty map
// If there was an issue with the retrieval, returns an error
func (d *SyncServerDatasource) GetAccountDataInRange(ctx context.Context, userID string, r types.Range, accountDataFilterPart *gomatrixserverlib.EventFilter) (map[string][]string, error) {
	return d.Database.GetAccountDataInRange(d.dbctx, userID, r, accountDataFilterPart)
}

// UpsertAccountData keeps track of new or updated account data, by saving the type
// of the new/updated data, and the user ID and room ID the data is related to (empty)
// room ID means the data isn't specific to any room)
// If no data with the given type, user ID and room ID exists in the database,
// creates a new row, else update the existing one
// Returns an error if there was an issue with the upsert
func (d *SyncServerDatasource) UpsertAccountData(ctx context.Context, userID, roomID, dataType string) (types.StreamPosition, error) {
	return d.Database.UpsertAccountData(d.dbctx, userID, roomID, dataType)
}

// AddInviteEvent stores a new invite event for a user.
// If the invite was successfully stored this returns the stream ID it was stored at.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatasource) AddInviteEvent(ctx context.Context, inviteEvent gomatrixserverlib.HeaderedEvent) (types.StreamPosition, error) {
	return d.Database.AddInviteEvent(d.dbctx, inviteEvent)
}

// RetireInviteEvent removes an old invite event from the database. Returns the new position of the retired invite.
// Returns an error if there was a problem communicating with the database.
func (d *SyncServerDatasource) RetireInviteEvent(ctx context.Context, inviteEventID string) (types.StreamPosition, error) {
	return d.Database.RetireInviteEvent(d.dbctx, inviteEventID)
}

// GetEventsInStreamingRange retrieves all of the events on a given ordering using the given extremities and limit.
func (d *SyncServerDatasource) GetEventsInStreamingRange(ctx context.Context, from, to *types.StreamingToken, roomID string, limit int, backwardOrdering bool) (events []types.StreamEvent, err error) {
	return d.Database.GetEventsInStreamingRange(d.dbctx, from, to, roomID, limit, backwardOrdering)
}

// GetEventsInTopologicalRange retrieves all of the events on a given ordering using the given extremities and limit.
func (d *SyncServerDatasource) GetEventsInTopologicalRange(ctx context.Context, from, to *types.TopologyToken, roomID string, limit int, backwardOrdering bool) (events []types.StreamEvent, err error) {
	return d.Database.GetEventsInTopologicalRange(d.dbctx, from, to, roomID, limit, backwardOrdering)
}

// EventPositionInTopology returns the depth and stream position of the given event.
func (d *SyncServerDatasource) EventPositionInTopology(ctx context.Context, eventID string) (types.TopologyToken, error) {
	return d.Database.EventPositionInTopology(d.dbctx, eventID)
}

// BackwardExtremitiesForRoom returns a map of backwards extremity event ID to a list of its prev_events.
func (d *SyncServerDatasource) BackwardExtremitiesForRoom(ctx context.Context, roomID string) (backwardExtremities map[string][]string, err error) {
	return d.Database.BackwardExtremitiesForRoom(d.dbctx, roomID)
}

func (d *SyncServerDatasource) MaxTopologicalPosition(ctx context.Context, roomID string) (types.TopologyToken, error) {
	return d.Database.MaxTopologicalPosition(d.dbctx, roomID)
}

// SendToDeviceUpdatesForSync returns a list of send-to-device updates. It returns three lists:
// - "events": a list of send-to-device events that should be included in the sync
// - "changes": a list of send-to-device events that should be updated in the database by
//      CleanSendToDeviceUpdates
// - "deletions": a list of send-to-device events which have been confirmed as sent and
//      can be deleted altogether by CleanSendToDeviceUpdates
// The token supplied should be the current requested sync token, e.g. from the "since"
// parameter.
func (d *SyncServerDatasource) SendToDeviceUpdatesForSync(ctx context.Context, userID, deviceID string, token types.StreamingToken) (events []types.SendToDeviceEvent, changes []types.SendToDeviceNID, deletions []types.SendToDeviceNID, err error) {
	return d.Database.SendToDeviceUpdatesForSync(d.dbctx, userID, deviceID, token)
}

// StoreNewSendForDeviceMessage stores a new send-to-device event for a user's device.
func (d *SyncServerDatasource) StoreNewSendForDeviceMessage(ctx context.Context, streamPos types.StreamPosition, userID, deviceID string, event gomatrixserverlib.SendToDeviceEvent) (types.StreamPosition, error) {
	return d.Database.StoreNewSendForDeviceMessage(d.dbctx, streamPos, userID, deviceID, event)
}

// CleanSendToDeviceUpdates will update or remove any send-to-device updates based on the
// result to a previous call to SendDeviceUpdatesForSync. This is separate as it allows
// SendToDeviceUpdatesForSync to be called multiple times if needed (e.g. before and after
// starting to wait for an incremental sync with timeout).
// The token supplied should be the current requested sync token, e.g. from the "since"
// parameter.
func (d *SyncServerDatasource) CleanSendToDeviceUpdates(ctx context.Context, toUpdate, toDelete []types.SendToDeviceNID, token types.StreamingToken) (err error) {
	return d.Database.CleanSendToDeviceUpdates(d.dbctx, toUpdate, toDelete, token)
}

// SendToDeviceUpdatesWaiting returns true if there are send-to-device updates waiting to be sent.
func (d *SyncServerDatasource) SendToDeviceUpdatesWaiting(ctx context.Context, userID, deviceID string) (bool, error) {
	return d.Database.SendToDeviceUpdatesWaiting(d.dbctx, userID, deviceID)
}

// GetFilter looks up the filter associated with a given local user and filter ID.
// Returns a filter structure. Otherwise returns an error if no such filter exists
// or if there was an error talking to the database.
func (d *SyncServerDatasource) GetFilter(ctx context.Context, localpart string, filterID string) (*gomatrixserverlib.Filter, error) {
	return d.Database.GetFilter(d.dbctx, localpart, filterID)
}

// PutFilter puts the passed filter into the database.
// Returns the filterID as a string. Otherwise returns an error if something
// goes wrong.
func (d *SyncServerDatasource) PutFilter(ctx context.Context, localpart string, filter *gomatrixserverlib.Filter) (string, error) {
	return d.Database.PutFilter(d.dbctx, localpart, filter)
}

// RedactEvent wipes an event in the database and sets the unsigned.redacted_because key to the redaction event
func (d *SyncServerDatasource) RedactEvent(ctx context.Context, redactedEventID string, redactedBecause *gomatrixserverlib.HeaderedEvent) error {
	return d.Database.RedactEvent(d.dbctx, redactedEventID, redactedBecause)
}
