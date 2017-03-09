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
