package api

import "github.com/matrix-org/gomatrixserverlib"

const (
	MSigningKeyUpdate = "m.signing_key_update"
	MTyping           = "m.typing"
	MReceipt          = "m.receipt"
)

type TypingEvent struct {
	Type   string `json:"type"`
	RoomID string `json:"room_id"`
	UserID string `json:"user_id"`
	Typing bool   `json:"typing"`
}

type ReceiptEvent struct {
	UserID    string                      `json:"user_id"`
	RoomID    string                      `json:"room_id"`
	EventID   string                      `json:"event_id"`
	Type      string                      `json:"type"`
	Timestamp gomatrixserverlib.Timestamp `json:"timestamp"`
}

type FederationReceiptMRead struct {
	User map[string]FederationReceiptData `json:"m.read"`
}

type FederationReceiptData struct {
	Data     ReceiptTS `json:"data"`
	EventIDs []string  `json:"event_ids"`
}

type ReceiptMRead struct {
	User map[string]ReceiptTS `json:"m.read"`
}

type ReceiptTS struct {
	TS gomatrixserverlib.Timestamp `json:"ts"`
}

type CrossSigningKeyUpdate struct {
	MasterKey      *gomatrixserverlib.CrossSigningKey `json:"master_key,omitempty"`
	SelfSigningKey *gomatrixserverlib.CrossSigningKey `json:"self_signing_key,omitempty"`
	UserID         string                             `json:"user_id"`
}
