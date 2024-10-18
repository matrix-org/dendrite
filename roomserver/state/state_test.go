// Copyright 2024 New Vector Ltd.
// Copyright 2019, 2020 The Matrix.org Foundation C.I.C.
// Copyright 2018 New Vector Ltd
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package state

import (
	"testing"

	"github.com/element-hq/dendrite/roomserver/types"
)

func TestFindDuplicateStateKeys(t *testing.T) {
	testCases := []struct {
		Input []types.StateEntry
		Want  []types.StateEntry
	}{{
		Input: []types.StateEntry{
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 1}, EventNID: 1},
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 1}, EventNID: 2},
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 2, EventStateKeyNID: 2}, EventNID: 3},
		},
		Want: []types.StateEntry{
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 1}, EventNID: 1},
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 1}, EventNID: 2},
		},
	}, {
		Input: []types.StateEntry{
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 1}, EventNID: 1},
			{StateKeyTuple: types.StateKeyTuple{EventTypeNID: 1, EventStateKeyNID: 2}, EventNID: 2},
		},
		Want: nil,
	}}

	for _, test := range testCases {
		got := findDuplicateStateKeys(test.Input)
		if len(got) != len(test.Want) {
			t.Fatalf("Wanted %v, got %v", test.Want, got)
		}
		for i := range got {
			if got[i] != test.Want[i] {
				t.Fatalf("Wanted %v, got %v", test.Want, got)
			}
		}
	}
}
