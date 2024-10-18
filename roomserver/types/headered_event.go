// Copyright 2024 New Vector Ltd.
// Copyright 2023 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

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
