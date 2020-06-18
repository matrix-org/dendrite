package api

import (
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type PerformJoinRequest struct {
	RoomIDOrAlias string                         `json:"room_id_or_alias"`
	UserID        string                         `json:"user_id"`
	Content       map[string]interface{}         `json:"content"`
	ServerNames   []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformJoinResponse struct {
	RoomID string `json:"room_id"`
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformLeaveResponse struct {
}

// PerformBackfillRequest is a request to PerformBackfill.
type PerformBackfillRequest struct {
	// The room to backfill
	RoomID string `json:"room_id"`
	// A map of backwards extremity event ID to a list of its prev_event IDs.
	BackwardsExtremities map[string][]string `json:"backwards_extremities"`
	// The maximum number of events to retrieve.
	Limit int `json:"limit"`
	// The server interested in the events.
	ServerName gomatrixserverlib.ServerName `json:"server_name"`
}

// PrevEventIDs returns the prev_event IDs of all backwards extremities, de-duplicated in a lexicographically sorted order.
func (r *PerformBackfillRequest) PrevEventIDs() []string {
	var prevEventIDs []string
	for _, pes := range r.BackwardsExtremities {
		prevEventIDs = append(prevEventIDs, pes...)
	}
	prevEventIDs = util.UniqueStrings(prevEventIDs)
	return prevEventIDs
}

// PerformBackfillResponse is a response to PerformBackfill.
type PerformBackfillResponse struct {
	// Missing events, arbritrary order.
	Events []gomatrixserverlib.HeaderedEvent `json:"events"`
}
