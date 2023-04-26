package types

import (
	"github.com/matrix-org/gomatrixserverlib"
)

// HeaderedEvent is an Event which serialises to the headered form, which includes
// _room_version and _event_id fields.
type HeaderedEvent struct {
	*gomatrixserverlib.Event
	Visibility gomatrixserverlib.HistoryVisibility
}

func (h *HeaderedEvent) MarshalJSON() ([]byte, error) {
	return h.Event.ToHeaderedJSON()
}

func (j *HeaderedEvent) UnmarshalJSON(data []byte) error {
	ev, err := gomatrixserverlib.NewEventFromHeaderedJSON(data, false)
	if err != nil {
		return err
	}
	j.Event = ev
	return nil
}

func NewEventJSONsFromHeaderedEvents(hes []*HeaderedEvent) gomatrixserverlib.EventJSONs {
	result := make(gomatrixserverlib.EventJSONs, len(hes))
	for i := range hes {
		result[i] = hes[i].JSON()
	}
	return result
}
