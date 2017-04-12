package events

import "encoding/json"

// ClientEvent is an event which is fit for consumption by clients, in accordance with the specification.
type ClientEvent struct {
	Content        json.RawMessage `json:"content"`
	Sender         string          `json:"sender"`
	Type           string          `json:"type"`
	StateKey       *string         `json:"state_key,omitempty"`
	Unsigned       json.RawMessage `json:"unsigned"`
	OriginServerTS int64           `json:"origin_server_ts"`
	EventID        string          `json:"event_id"`
}
