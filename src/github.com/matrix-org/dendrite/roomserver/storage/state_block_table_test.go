// Copyright 2017 Vector Creations Ltd
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

package storage

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"sort"
	"testing"
)

func TestStateKeyTupleSorter(t *testing.T) {
	input := stateKeyTupleSorter{
		{1, 2},
		{1, 4},
		{2, 2},
		{1, 1},
	}
	want := []types.StateKeyTuple{
		{1, 1},
		{1, 2},
		{1, 4},
		{2, 2},
	}
	doNotWant := []types.StateKeyTuple{
		{0, 0},
		{1, 3},
		{2, 1},
		{3, 1},
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
