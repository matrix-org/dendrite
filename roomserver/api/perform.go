package api

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/fclient"
	"github.com/matrix-org/gomatrixserverlib/spec"
	"github.com/matrix-org/util"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
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
	default:
		return util.ErrorResponse(p)
	}
}

const (
	// PerformErrorNotAllowed means the user is not allowed to invite/join/etc this room (e.g join_rule:invite or banned)
	PerformErrorNotAllowed PerformErrorCode = 1
	// PerformErrorNoRoom means that the room being joined doesn't exist.
	PerformErrorNoRoom PerformErrorCode = 3
)

type PerformJoinRequest struct {
	RoomIDOrAlias string                 `json:"room_id_or_alias"`
	UserID        string                 `json:"user_id"`
	IsGuest       bool                   `json:"is_guest"`
	Content       map[string]interface{} `json:"content"`
	ServerNames   []spec.ServerName      `json:"server_names"`
	Unsigned      map[string]interface{} `json:"unsigned"`
}

type PerformLeaveRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformLeaveResponse struct {
	Code    int         `json:"code,omitempty"`
	Message interface{} `json:"message,omitempty"`
}

type PerformInviteRequest struct {
	RoomVersion     gomatrixserverlib.RoomVersion    `json:"room_version"`
	Event           *gomatrixserverlib.HeaderedEvent `json:"event"`
	InviteRoomState []fclient.InviteV2StrippedState  `json:"invite_room_state"`
	SendAsServer    string                           `json:"send_as_server"`
	TransactionID   *TransactionID                   `json:"transaction_id"`
}

type PerformPeekRequest struct {
	RoomIDOrAlias string            `json:"room_id_or_alias"`
	UserID        string            `json:"user_id"`
	DeviceID      string            `json:"device_id"`
	ServerNames   []spec.ServerName `json:"server_names"`
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
	ServerName spec.ServerName `json:"server_name"`
	// Which virtual host are we doing this for?
	VirtualHost spec.ServerName `json:"virtual_host"`
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
	Events            []*gomatrixserverlib.HeaderedEvent  `json:"events"`
	HistoryVisibility gomatrixserverlib.HistoryVisibility `json:"history_visibility"`
}

type PerformPublishRequest struct {
	RoomID       string
	Visibility   string
	AppserviceID string
	NetworkID    string
}

type PerformInboundPeekRequest struct {
	UserID          string          `json:"user_id"`
	RoomID          string          `json:"room_id"`
	PeekID          string          `json:"peek_id"`
	ServerName      spec.ServerName `json:"server_name"`
	RenewalInterval int64           `json:"renewal_interval"`
}

type PerformInboundPeekResponse struct {
	// Does the room exist on this roomserver?
	// If the room doesn't exist this will be false and StateEvents will be empty.
	RoomExists bool `json:"room_exists"`
	// The room version of the room.
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
	// The current state and auth chain events.
	// The lists will be in an arbitrary order.
	StateEvents     []*gomatrixserverlib.HeaderedEvent `json:"state_events"`
	AuthChainEvents []*gomatrixserverlib.HeaderedEvent `json:"auth_chain_events"`
	// The event at which this state was captured
	LatestEvent *gomatrixserverlib.HeaderedEvent `json:"latest_event"`
}

// PerformForgetRequest is a request to PerformForget
type PerformForgetRequest struct {
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
}

type PerformForgetResponse struct{}

type PerformRoomUpgradeRequest struct {
	RoomID      string                        `json:"room_id"`
	UserID      string                        `json:"user_id"`
	RoomVersion gomatrixserverlib.RoomVersion `json:"room_version"`
}

type PerformRoomUpgradeResponse struct {
	NewRoomID string
	Error     *PerformError
}
