package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

type PerformErrorCode int

type PerformError struct {
	Msg        string
	RemoteCode int // remote HTTP status code, for PerformErrRemote
	Code       PerformErrorCode
}

func (p *PerformError) Error() string {
	return fmt.Sprintf("%d : %s", p.Code, p.Msg)
}

// JSONResponse maps error codes to suitable HTTP error codes, defaulting to 500.
func (p *PerformError) JSONResponse() util.JSONResponse {
	switch p.Code {
	case PerformErrorBadRequest:
		return util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.Unknown(p.Msg),
		}
	case PerformErrorNoRoom:
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound(p.Msg),
		}
	case PerformErrorNotAllowed:
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(p.Msg),
		}
	case PerformErrorNoOperation:
		return util.JSONResponse{
			Code: http.StatusForbidden,
			JSON: jsonerror.Forbidden(p.Msg),
		}
	case PerformErrRemote:
		// if the code is 0 then something bad happened and it isn't
		// a remote HTTP error being encapsulated, e.g network error to remote.
		if p.RemoteCode == 0 {
			return util.ErrorResponse(fmt.Errorf("%s", p.Msg))
		}
		return util.JSONResponse{
			Code: p.RemoteCode,
			// TODO: Should we assert this is in fact JSON? E.g gjson parse?
			JSON: json.RawMessage(p.Msg),
		}
	default:
		return util.ErrorResponse(p)
	}
}

const (
	// PerformErrorNotAllowed means the user is not allowed to invite/join/etc this room (e.g join_rule:invite or banned)
	PerformErrorNotAllowed PerformErrorCode = 1
	// PerformErrorBadRequest means the request was wrong in some way (invalid user ID, wrong server, etc)
	PerformErrorBadRequest PerformErrorCode = 2
	// PerformErrorNoRoom means that the room being joined doesn't exist.
	PerformErrorNoRoom PerformErrorCode = 3
	// PerformErrorNoOperation means that the request resulted in nothing happening e.g invite->invite or leave->leave.
	PerformErrorNoOperation PerformErrorCode = 4
	// PerformErrRemote means that the request failed and the PerformError.Msg is the raw remote JSON error response
	PerformErrRemote PerformErrorCode = 5
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
	// If non-nil, the join request failed. Contains more information why it failed.
	Error *PerformError
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformLeaveResponse struct {
}

type PerformInviteRequest struct {
	RoomVersion     gomatrixserverlib.RoomVersion             `json:"room_version"`
	Event           gomatrixserverlib.HeaderedEvent           `json:"event"`
	InviteRoomState []gomatrixserverlib.InviteV2StrippedState `json:"invite_room_state"`
	SendAsServer    string                                    `json:"send_as_server"`
	TransactionID   *TransactionID                            `json:"transaction_id"`
}

type PerformInviteResponse struct {
	// If non-nil, the invite request failed. Contains more information why it failed.
	Error *PerformError
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

type PerformPublishRequest struct {
	RoomID     string
	Visibility string
}

type PerformPublishResponse struct {
	// If non-nil, the publish request failed. Contains more information why it failed.
	Error *PerformError
}
