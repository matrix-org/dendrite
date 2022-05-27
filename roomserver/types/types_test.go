package types

import (
	"sort"
	"testing"
)

func TestDeduplicateStateEntries(t *testing.T) {
	entries := []StateEntry{
		{StateKeyTuple{1, 1}, 1},
		{StateKeyTuple{1, 1}, 2},
		{StateKeyTuple{1, 1}, 3},
		{StateKeyTuple{2, 2}, 4},
		{StateKeyTuple{2, 3}, 5},
		{StateKeyTuple{3, 3}, 6},
	}
	expected := []EventNID{3, 4, 5, 6}
	entries = DeduplicateStateEntries(entries)
	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries, got %d entries", len(entries))
	}
	for i, v := range entries {
		if v.EventNID != expected[i] {
			t.Fatalf("Expected position %d to be %d but got %d", i, expected[i], v.EventNID)
		}
	}
}

func TestStateKeyTupleSorter(t *testing.T) {
	input := StateKeyTupleSorter{
		{EventTypeNID: 1, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 4},
		{EventTypeNID: 2, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 1},
	}
	want := []StateKeyTuple{
		{EventTypeNID: 1, EventStateKeyNID: 1},
		{EventTypeNID: 1, EventStateKeyNID: 2},
		{EventTypeNID: 1, EventStateKeyNID: 4},
		{EventTypeNID: 2, EventStateKeyNID: 2},
	}
	doNotWant := []StateKeyTuple{
		{EventTypeNID: 0, EventStateKeyNID: 0},
		{EventTypeNID: 1, EventStateKeyNID: 3},
		{EventTypeNID: 2, EventStateKeyNID: 1},
		{EventTypeNID: 3, EventStateKeyNID: 1},
	}
	wantTypeNIDs := []int64{1, 2}
	wantStateKeyNIDs := []int64{1, 2, 4}

	// Sort the input and check it's in the right order.
	sort.Sort(input)
	gotTypeNIDs, gotStateKeyNIDs := input.TypesAndStateKeysAsArrays()

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
