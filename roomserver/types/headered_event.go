// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"unsafe"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// HeaderedEvent is an Event which serialises to the headered form, which includes
// _room_version and _event_id fields.
type HeaderedEvent struct {
	gomatrixserverlib.PDU
	Visibility gomatrixserverlib.HistoryVisibility
	// TODO: Remove this. This is a temporary workaround to store the userID in the syncAPI.
	// 		It really should be the userKey instead.
	UserID           spec.UserID
	StateKeyResolved *string
}

func (h *HeaderedEvent) CacheCost() int {
	return int(unsafe.Sizeof(*h)) +
		len(h.EventID()) +
		(cap(h.JSON()) * 2) +
		len(h.Version()) +
		1 // redacted bool
}

func (h *HeaderedEvent) MarshalJSON() ([]byte, error) {
	return h.PDU.ToHeaderedJSON()
}

func (j *HeaderedEvent) UnmarshalJSON(data []byte) error {
	ev, err := gomatrixserverlib.NewEventFromHeaderedJSON(data, false)
	if err != nil {
		return err
	}
	j.PDU = ev
	return nil
}

func NewEventJSONsFromHeaderedEvents(hes []*HeaderedEvent) gomatrixserverlib.EventJSONs {
	result := make(gomatrixserverlib.EventJSONs, len(hes))
	for i := range hes {
		result[i] = hes[i].JSON()
	}
	return result
}
