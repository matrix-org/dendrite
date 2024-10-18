// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

// Package api provides the types that are used to communicate with the roomserver.
package api

import (
	"fmt"

	"github.com/element-hq/dendrite/roomserver/types"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

type Kind int

const (
	// KindOutlier event fall outside the contiguous event graph.
	// We do not have the state for these events.
	// These events are state events used to authenticate other events.
	// They can become part of the contiguous event graph via backfill.
	KindOutlier Kind = iota + 1
	// KindNew event extend the contiguous graph going forwards.
	// They usually don't need state, but may include state if the
	// there was a new event that references an event that we don't
	// have a copy of. New events will influence the fwd extremities
	// of the room and output events will be generated as a result.
	KindNew
	// KindOld event extend the graph backwards, or fill gaps in
	// history. They may or may not include state. They will not be
	// considered for forward extremities, and output events will NOT
	// be generated for them.
	KindOld
)

func (k Kind) String() string {
	switch k {
	case KindOutlier:
		return "KindOutlier"
	case KindNew:
		return "KindNew"
	case KindOld:
		return "KindOld"
	default:
		return "(unknown)"
	}
}

// DoNotSendToOtherServers tells us not to send the event to other matrix
// servers.
const DoNotSendToOtherServers = ""

// InputRoomEvent is a matrix room event to add to the room server database.
// TODO: Implement UnmarshalJSON/MarshalJSON in a way that does something sensible with the event JSON.
type InputRoomEvent struct {
	// Whether this event is new, backfilled or an outlier.
	// This controls how the event is processed.
	Kind Kind `json:"kind"`
	// The event JSON for the event to add.
	Event *types.HeaderedEvent `json:"event"`
	// Which server told us about this event.
	Origin spec.ServerName `json:"origin"`
	// Whether the state is supplied as a list of event IDs or whether it
	// should be derived from the state at the previous events.
	HasState bool `json:"has_state"`
	// Optional list of state event IDs forming the state before this event.
	// These state events must have already been persisted.
	// These are only used if HasState is true.
	// The list can be empty, for example when storing the first event in a room.
	StateEventIDs []string `json:"state_event_ids"`
	// The server name to use to push this event to other servers.
	// Or empty if this event shouldn't be pushed to other servers.
	SendAsServer string `json:"send_as_server"`
	// The transaction ID of the send request if sent by a local user and one
	// was specified
	TransactionID *TransactionID `json:"transaction_id"`
}

// TransactionID contains the transaction ID sent by a client when sending an
// event, along with the ID of the client session.
type TransactionID struct {
	SessionID     int64  `json:"session_id"`
	TransactionID string `json:"id"`
}

// InputRoomEventsRequest is a request to InputRoomEvents
type InputRoomEventsRequest struct {
	InputRoomEvents []InputRoomEvent `json:"input_room_events"`
	Asynchronous    bool             `json:"async"`
	VirtualHost     spec.ServerName  `json:"virtual_host"`
}

// InputRoomEventsResponse is a response to InputRoomEvents
type InputRoomEventsResponse struct {
	ErrMsg     string // set if there was any error
	NotAllowed bool   // true if an event in the input was not allowed.
}

func (r *InputRoomEventsResponse) Err() error {
	if r.ErrMsg == "" {
		return nil
	}
	if r.NotAllowed {
		return &gomatrixserverlib.NotAllowed{
			Message: r.ErrMsg,
		}
	}
	return fmt.Errorf("InputRoomEventsResponse: %s", r.ErrMsg)
}
