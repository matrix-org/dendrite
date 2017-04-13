package events

import (
	"encoding/json"
	"github.com/matrix-org/gomatrixserverlib"
)

// ClientEvent is an event which is fit for consumption by clients, in accordance with the specification.
type ClientEvent struct {
	Content        json.RawMessage `json:"content"`
	Sender         string          `json:"sender"`
	Type           string          `json:"type"`
	StateKey       *string         `json:"state_key,omitempty"`
	Unsigned       json.RawMessage `json:"unsigned,omitempty"`
	OriginServerTS int64           `json:"origin_server_ts"`
	EventID        string          `json:"event_id"`
}

// ToClientEvents converts server events to client events
func ToClientEvents(serverEvs []gomatrixserverlib.Event) []ClientEvent {
	evs := make([]ClientEvent, len(serverEvs))
	for i, se := range serverEvs {
		evs[i] = ToClientEvent(se)
	}
	return evs
}

// ToClientEvent converts a single server event to a client event
func ToClientEvent(se gomatrixserverlib.Event) ClientEvent {
	return ClientEvent{
		Content:  json.RawMessage(se.Content()),
		Sender:   se.Sender(),
		Type:     se.Type(),
		StateKey: se.StateKey(),
		// TODO: No unsigned / origin_server_ts fields?
		EventID: se.EventID(),
	}
}
