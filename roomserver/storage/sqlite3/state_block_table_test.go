// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package sqlite3

import (
	"sort"
	"testing"

	"github.com/matrix-org/dendrite/roomserver/types"
)

func TestStateKeyTupleSorter(t *testing.T) {
	input := stateKeyTupleSorter{
		{EventTypeNID: 1, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 4},
		{EventTypeNID: 2, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 1},
	}
	want := []types.StateKeyTuple{
		{EventTypeNID: 1, EventStateKeyNID: 1},
		{EventTypeNID: 1, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 4},
		{EventTypeNID: 2, EventStateKeyNID: 2},
	}
	doNotWant := []types.StateKeyTuple{
		{EventTypeNID: 0, EventStateKeyNID: 0},
		{EventTypeNID: 1, EventStateKeyNID: 3},
		{EventTypeNID: 2, EventStateKeyNID: 1},
		{EventTypeNID: 3, EventStateKeyNID: 1},
	}
	wantTypeNIDs := []int64{1, 2}
	wantStateKeyNIDs := []int64{1, 2, 4}

	// Sort the input and check it's in the right order.
	sort.Sort(input)
	gotTypeNIDs, gotStateKeyNIDs := input.typesAndStateKeysAsArrays()

	for i := range want {
		if input[i] != want[i] {
			t.Errorf("Wanted %#v at index %d got %#v", want[i], i, input[i])
		}

		if !input.contains(want[i]) {
			t.Errorf("Wanted %#v.contains(%#v) to be true but got false", input, want[i])
		}
	}

	for i := range doNotWant {
		if input.contains(doNotWant[i]) {
			t.Errorf("Wanted %#v.contains(%#v) to be false but got true", input, doNotWant[i])
		}
	}

	if len(wantTypeNIDs) != len(gotTypeNIDs) {
		t.Fatalf("Wanted type NIDs %#v got %#v", wantTypeNIDs, gotTypeNIDs)
	}

	for i := range wantTypeNIDs {
		if wantTypeNIDs[i] != gotTypeNIDs[i] {
			t.Fatalf("Wanted type NIDs %#v got %#v", wantTypeNIDs, gotTypeNIDs)
		}
	}

	if len(wantStateKeyNIDs) != len(gotStateKeyNIDs) {
		t.Fatalf("Wanted state key NIDs %#v got %#v", wantStateKeyNIDs, gotStateKeyNIDs)
	}

	for i := range wantStateKeyNIDs {
		if wantStateKeyNIDs[i] != gotStateKeyNIDs[i] {
			t.Fatalf("Wanted type NIDs %#v got %#v", wantTypeNIDs, gotTypeNIDs)
		}
	}
}
