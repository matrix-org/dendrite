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

package storage

import (
	"context"
	"time"

	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/internal"
	"github.com/matrix-org/dendrite/roomserver/api"
	"github.com/matrix-org/dendrite/syncapi/types"
	userapi "github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib"
)

type Database interface {
	internal.PartitionStorer
	// AllJoinedUsersInRooms returns a map of room ID to a list of all joined user IDs.
	AllJoinedUsersInRooms(ctx context.Context) (map[string][]string, error)
	// Events lookups a list of event by their event ID.
	// Returns a list of events matching the requested IDs found in the database.
	// If an event is not found in the database then it will be omitted from the list.
	// Returns an error if there was a problem talking with the database.
	// Does not include any transaction IDs in the returned events.
	Events(ctx context.Context, eventIDs []string) ([]gomatrixserverlib.HeaderedEvent, error)
	// WriteEvent into the database. It is not safe to call this function from multiple goroutines, as it would create races
	// when generating the sync stream position for this event. Returns the sync stream position for the inserted event.
	// Returns an error if there was a problem inserting this event.
	WriteEvent(ctx context.Context, ev *gomatrixserverlib.HeaderedEvent, addStateEvents []gomatrixserverlib.HeaderedEvent,
		addStateEventIDs []string, removeStateEventIDs []string, transactionID *api.TransactionID, excludeFromSync bool) (types.StreamPosition, error)
	// GetStateEvent returns the Matrix state event of a given type for a given room with a given state key
	// If no event could be found, returns nil
	// If there was an issue during the retrieval, returns an error
	GetStateEvent(ctx context.Context, roomID, evType, stateKey string) (*gomatrixserverlib.HeaderedEvent, error)
	// GetStateEventsForRoom fetches the state events for a given room.
	// Returns an empty slice if no state events could be found for this room.
	// Returns an error if there was an issue with the retrieval.
	GetStateEventsForRoom(ctx context.Context, roomID string, stateFilterPart *gomatrixserverlib.StateFilter) (stateEvents []gomatrixserverlib.HeaderedEvent, err error)
	// SyncPosition returns the latest positions for syncing.
	SyncPosition(ctx context.Context) (types.StreamingToken, error)
	// IncrementalSync returns all the data needed in order to create an incremental
	// sync response for the given user. Events returned will include any client
	// transaction IDs associated with the given device. These transaction IDs come
	// from when the device sent the event via an API that included a transaction
	// ID. A response object must be provided for IncrementaSync to populate - it
	// will not create one.
	IncrementalSync(ctx context.Context, res *types.Response, device userapi.Device, fromPos, toPos types.StreamingToken, numRecentEventsPerRoom int, wantFullState bool) (*types.Response, error)
	// CompleteSync returns a complete /sync API response for the given user. A response object
	// must be provided for CompleteSync to populate - it will not create one.
	CompleteSync(ctx context.Context, res *types.Response, device userapi.Device, numRecentEventsPerRoom int) (*types.Response, error)
	// GetAccountDataInRange returns all account data for a given user inserted or
	// updated between two given positions
	// Returns a map following the format data[roomID] = []dataTypes
	// If no data is retrieved, returns an empty map
	// If there was an issue with the retrieval, returns an error
	GetAccountDataInRange(ctx context.Context, userID string, r types.Range, accountDataFilterPart *gomatrixserverlib.EventFilter) (map[string][]string, error)
	// UpsertAccountData keeps track of new or updated account data, by saving the type
	// of the new/updated data, and the user ID and room ID the data is related to (empty)
	// room ID means the data isn't specific to any room)
	// If no data with the given type, user ID and room ID exists in the database,
	// creates a new row, else update the existing one
	// Returns an error if there was an issue with the upsert
	UpsertAccountData(ctx context.Context, userID, roomID, dataType string) (types.StreamPosition, error)
	// AddInviteEvent stores a new invite event for a user.
	// If the invite was successfully stored this returns the stream ID it was stored at.
	// Returns an error if there was a problem communicating with the database.
	AddInviteEvent(ctx context.Context, inviteEvent gomatrixserverlib.HeaderedEvent) (types.StreamPosition, error)
	// RetireInviteEvent removes an old invite event from the database. Returns the new position of the retired invite.
	// Returns an error if there was a problem communicating with the database.
	RetireInviteEvent(ctx context.Context, inviteEventID string) (types.StreamPosition, error)
	// SetTypingTimeoutCallback sets a callback function that is called right after
	// a user is removed from the typing user list due to timeout.
	SetTypingTimeoutCallback(fn cache.TimeoutCallbackFn)
	// AddTypingUser adds a typing user to the typing cache.
	// Returns the newly calculated sync position for typing notifications.
	AddTypingUser(userID, roomID string, expireTime *time.Time) types.StreamPosition
	// RemoveTypingUser removes a typing user from the typing cache.
	// Returns the newly calculated sync position for typing notifications.
	RemoveTypingUser(userID, roomID string) types.StreamPosition
	// GetEventsInStreamingRange retrieves all of the events on a given ordering using the given extremities and limit.
	GetEventsInStreamingRange(ctx context.Context, from, to *types.StreamingToken, roomID string, limit int, backwardOrdering bool) (events []types.StreamEvent, err error)
	// GetEventsInTopologicalRange retrieves all of the events on a given ordering using the given extremities and limit.
	GetEventsInTopologicalRange(ctx context.Context, from, to *types.TopologyToken, roomID string, limit int, backwardOrdering bool) (events []types.StreamEvent, err error)
	// EventPositionInTopology returns the depth and stream position of the given event.
	EventPositionInTopology(ctx context.Context, eventID string) (types.TopologyToken, error)
	// BackwardExtremitiesForRoom returns a map of backwards extremity event ID to a list of its prev_events.
	BackwardExtremitiesForRoom(ctx context.Context, roomID string) (backwardExtremities map[string][]string, err error)
	// MaxTopologicalPosition returns the highest topological position for a given room.
	MaxTopologicalPosition(ctx context.Context, roomID string) (types.TopologyToken, error)
	// StreamEventsToEvents converts streamEvent to Event. If device is non-nil and
	// matches the streamevent.transactionID device then the transaction ID gets
	// added to the unsigned section of the output event.
	StreamEventsToEvents(device *userapi.Device, in []types.StreamEvent) []gomatrixserverlib.HeaderedEvent
	// SyncStreamPosition returns the latest position in the sync stream. Returns 0 if there are no events yet.
	SyncStreamPosition(ctx context.Context) (types.StreamPosition, error)
	// AddSendToDevice increases the EDU position in the cache and returns the stream position.
	AddSendToDevice() types.StreamPosition
	// SendToDeviceUpdatesForSync returns a list of send-to-device updates. It returns three lists:
	// - "events": a list of send-to-device events that should be included in the sync
	// - "changes": a list of send-to-device events that should be updated in the database by
	//      CleanSendToDeviceUpdates
	// - "deletions": a list of send-to-device events which have been confirmed as sent and
	//      can be deleted altogether by CleanSendToDeviceUpdates
	// The token supplied should be the current requested sync token, e.g. from the "since"
	// parameter.
	SendToDeviceUpdatesForSync(ctx context.Context, userID, deviceID string, token types.StreamingToken) (events []types.SendToDeviceEvent, changes []types.SendToDeviceNID, deletions []types.SendToDeviceNID, err error)
	// StoreNewSendForDeviceMessage stores a new send-to-device event for a user's device.
	StoreNewSendForDeviceMessage(ctx context.Context, streamPos types.StreamPosition, userID, deviceID string, event gomatrixserverlib.SendToDeviceEvent) (types.StreamPosition, error)
	// CleanSendToDeviceUpdates will update or remove any send-to-device updates based on the
	// result to a previous call to SendDeviceUpdatesForSync. This is separate as it allows
	// SendToDeviceUpdatesForSync to be called multiple times if needed (e.g. before and after
	// starting to wait for an incremental sync with timeout).
	// The token supplied should be the current requested sync token, e.g. from the "since"
	// parameter.
	CleanSendToDeviceUpdates(ctx context.Context, toUpdate, toDelete []types.SendToDeviceNID, token types.StreamingToken) (err error)
	// SendToDeviceUpdatesWaiting returns true if there are send-to-device updates waiting to be sent.
	SendToDeviceUpdatesWaiting(ctx context.Context, userID, deviceID string) (bool, error)
	// GetFilter looks up the filter associated with a given local user and filter ID.
	// Returns a filter structure. Otherwise returns an error if no such filter exists
	// or if there was an error talking to the database.
	GetFilter(ctx context.Context, localpart string, filterID string) (*gomatrixserverlib.Filter, error)
	// PutFilter puts the passed filter into the database.
	// Returns the filterID as a string. Otherwise returns an error if something
	// goes wrong.
	PutFilter(ctx context.Context, localpart string, filter *gomatrixserverlib.Filter) (string, error)
	// RedactEvent wipes an event in the database and sets the unsigned.redacted_because key to the redaction event
	RedactEvent(ctx context.Context, redactedEventID string, redactedBecause *gomatrixserverlib.HeaderedEvent) error
}
