package api

import (
	"context"

	"github.com/matrix-org/dendrite/internal/pushrules"
	"github.com/matrix-org/gomatrixserverlib"
)

type PushserverInternalAPI interface {
	PerformPusherSet(ctx context.Context, req *PerformPusherSetRequest, res *struct{}) error
	PerformPusherDeletion(ctx context.Context, req *PerformPusherDeletionRequest, res *struct{}) error
	QueryPushers(ctx context.Context, req *QueryPushersRequest, res *QueryPushersResponse) error

	PerformPushRulesPut(ctx context.Context, req *PerformPushRulesPutRequest, res *struct{}) error
	QueryPushRules(ctx context.Context, req *QueryPushRulesRequest, res *QueryPushRulesResponse) error

	QueryNotifications(ctx context.Context, req *QueryNotificationsRequest, res *QueryNotificationsResponse) error
}

type QueryPushersRequest struct {
	Localpart string
}

type QueryPushersResponse struct {
	Pushers []Pusher `json:"pushers"`
}

type PerformPusherSetRequest struct {
	Pusher    // Anonymous field because that's how clientapi unmarshals it.
	Localpart string
	Append    bool `json:"append"`
}

type PerformPusherDeletionRequest struct {
	Localpart string
	SessionID int64
}

// Pusher represents a push notification subscriber
type Pusher struct {
	SessionID         int64                       `json:"session_id,omitempty"`
	PushKey           string                      `json:"pushkey"`
	PushKeyTS         gomatrixserverlib.Timestamp `json:"pushkey_ts,omitempty"`
	Kind              PusherKind                  `json:"kind"`
	AppID             string                      `json:"app_id"`
	AppDisplayName    string                      `json:"app_display_name"`
	DeviceDisplayName string                      `json:"device_display_name"`
	ProfileTag        string                      `json:"profile_tag"`
	Language          string                      `json:"lang"`
	Data              map[string]interface{}      `json:"data"`
}

type PusherKind string

const (
	EmailKind PusherKind = "email"
	HTTPKind  PusherKind = "http"
)

type PerformPushRulesPutRequest struct {
	UserID   string                     `json:"user_id"`
	RuleSets *pushrules.AccountRuleSets `json:"rule_sets"`
}

type QueryPushRulesRequest struct {
	UserID string `json:"user_id"`
}

type QueryPushRulesResponse struct {
	RuleSets *pushrules.AccountRuleSets `json:"rule_sets"`
}

type QueryNotificationsRequest struct {
	Localpart string `json:"localpart"` // Required.
	From      string `json:"from,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Only      string `json:"only,omitempty"`
}

type QueryNotificationsResponse struct {
	NextToken     string          `json:"next_token"`
	Notifications []*Notification `json:"notifications"` // Required.
}

type Notification struct {
	Actions    []*pushrules.Action           `json:"actions"`     // Required.
	Event      gomatrixserverlib.ClientEvent `json:"event"`       // Required.
	ProfileTag string                        `json:"profile_tag"` // Required by Sytest, but actually optional.
	Read       bool                          `json:"read"`        // Required.
	RoomID     string                        `json:"room_id"`     // Required.
	TS         gomatrixserverlib.Timestamp   `json:"ts"`          // Required.
}
