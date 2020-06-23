package api

import (
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type JoinError int

const (
	// JoinErrorNotAllowed means the user is not allowed to join this room (e.g join_rule:invite or banned)
	JoinErrorNotAllowed JoinError = 1
	// JoinErrorBadRequest means the request was wrong in some way (invalid user ID, wrong server, etc)
	JoinErrorBadRequest JoinError = 2
	// JoinErrorNoRoom means that the room being joined doesn't exist.
	JoinErrorNoRoom JoinError = 3
)

type PerformJoinRequest struct {
	RoomIDOrAlias string                         `json:"room_id_or_alias"`
	UserID        string                         `json:"user_id"`
	Content       map[string]interface{}         `json:"content"`
	ServerNames   []gomatrixserverlib.ServerName `json:"server_names"`
}

type PerformJoinResponse struct {
	// The room ID, populated on success.
	RoomID string `json:"room_id"`
	// The reason why the join failed. Can be blank.
	Error JoinError `json:"error"`
	// Debugging description of the error. Always present on failure.
	ErrMsg string `json:"err_msg"`
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
