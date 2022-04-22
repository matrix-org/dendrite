package pushgateway

import (
	"context"
	"encoding/json"
)

// A Client is how interactions with a Push Gateway is done.
type Client interface {
	// Notify sends a notification to the gateway at the given URL.
	Notify(ctx context.Context, url string, req *NotifyRequest, resp *NotifyResponse) error
}

type NotifyRequest struct {
	Notification Notification `json:"notification"` // Required
}

type NotifyResponse struct {
	// Rejected is the list of device push keys that were rejected
	// during the push. The caller should remove the push keys so they
	// are not used again.
	Rejected []string `json:"rejected"` // Required
}

type Notification struct {
	Content           json.RawMessage `json:"content,omitempty"`
	Counts            *Counts         `json:"counts,omitempty"`
	Devices           []*Device       `json:"devices"` // Required
	EventID           string          `json:"event_id,omitempty"`
	ID                string          `json:"id,omitempty"`         // Deprecated name for EventID.
	Membership        string          `json:"membership,omitempty"` // UNSPEC: required for Sytest.
	Prio              Prio            `json:"prio,omitempty"`
	RoomAlias         string          `json:"room_alias,omitempty"`
	RoomID            string          `json:"room_id,omitempty"`
	RoomName          string          `json:"room_name,omitempty"`
	Sender            string          `json:"sender,omitempty"`
	SenderDisplayName string          `json:"sender_display_name,omitempty"`
	Type              string          `json:"type,omitempty"`
	UserIsTarget      bool            `json:"user_is_target,omitempty"`
}

type Counts struct {
	MissedCalls int `json:"missed_calls,omitempty"`
	Unread      int `json:"unread"` // TODO: UNSPEC: the spec says zero must be omitted, but Sytest 61push/01message-pushed.pl requires it.
}

type Device struct {
	AppID     string                 `json:"app_id"`  // Required
	Data      map[string]interface{} `json:"data"`    // Required. UNSPEC: Sytests require this to allow unknown keys.
	PushKey   string                 `json:"pushkey"` // Required
	PushKeyTS int64                  `json:"pushkey_ts,omitempty"`
	Tweaks    map[string]interface{} `json:"tweaks,omitempty"`
}

type Prio string

const (
	HighPrio Prio = "high"
	LowPrio  Prio = "low"
)
