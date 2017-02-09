package input

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"testing"
)

func benchmarkStateEntryMapLookup(entries, lookups int64, b *testing.B) {
	var list []types.StateEntry
	for i := int64(0); i < entries; i++ {
		list = append(list, types.StateEntry{types.StateKeyTuple{i, i}, i})
	}

	for i := 0; i < b.N; i++ {
		entryMap := stateEntryMap(list)
		for j := int64(0); j < lookups; j++ {
			entryMap.lookup(types.StateKeyTuple{j, j})
		}
	}
}

func BenchmarkStateEntryMap100Lookup10(b *testing.B) {
	benchmarkStateEntryMapLookup(100, 10, b)
}

func BenchmarkStateEntryMap1000Lookup100(b *testing.B) {
	benchmarkStateEntryMapLookup(1000, 100, b)
}

func BenchmarkStateEntryMap100Lookup100(b *testing.B) {
	benchmarkStateEntryMapLookup(100, 100, b)
}

func BenchmarkStateEntryMap1000Lookup10000(b *testing.B) {
	benchmarkStateEntryMapLookup(1000, 10000, b)
}

func TestStateEntryMap(t *testing.T) {
	entryMap := stateEntryMap([]types.StateEntry{
		{types.StateKeyTuple{1, 1}, 1},
		{types.StateKeyTuple{1, 3}, 2},
		{types.StateKeyTuple{2, 1}, 3},
	})

	testCases := []struct {
		inputTypeNID  int64
		inputStateKey int64
		wantOK        bool
		wantEventNID  int64
	}{
		// Check that tuples that in the array are in the map.
		{1, 1, true, 1},
		{1, 3, true, 2},
		{2, 1, true, 3},
		// Check that tuples that aren't in the array aren't in the map.
		{0, 0, false, 0},
		{1, 2, false, 0},
		{3, 1, false, 0},
	}

	for _, testCase := range testCases {
		keyTuple := types.StateKeyTuple{testCase.inputTypeNID, testCase.inputStateKey}
		gotEventNID, gotOK := entryMap.lookup(keyTuple)
		if testCase.wantOK != gotOK {
			t.Fatalf("stateEntryMap lookup(%v): want ok to be %v, got %v", keyTuple, testCase.wantOK, gotOK)
		}
		if testCase.wantEventNID != gotEventNID {
			t.Fatalf("stateEntryMap lookup(%v): want eventNID to be %v, got %v", keyTuple, testCase.wantEventNID, gotEventNID)
		}
	}
}

func TestEventMap(t *testing.T) {
	events := eventMap([]types.Event{
		{EventNID: 1},
		{EventNID: 2},
		{EventNID: 3},
		{EventNID: 5},
		{EventNID: 8},
	})

	testCases := []struct {
		inputEventNID int64
		wantOK        bool
		wantEvent     *types.Event
	}{
		// Check that the IDs that are in the array are in the map.
		{1, true, &events[0]},
		{2, true, &events[1]},
		{3, true, &events[2]},
		{5, true, &events[3]},
		{8, true, &events[4]},
		// Check that tuples that aren't in the array aren't in the map.
		{0, false, nil},
		{4, false, nil},
		{6, false, nil},
		{7, false, nil},
		{9, false, nil},
	}

	for _, testCase := range testCases {
		gotEvent, gotOK := events.lookup(testCase.inputEventNID)
		if testCase.wantOK != gotOK {
			t.Fatalf("eventMap lookup(%v): want ok to be %v, got %v", testCase.inputEventNID, testCase.wantOK, gotOK)
		}

		if testCase.wantEvent != gotEvent {
			t.Fatalf("eventMap lookup(%v): want event to be %v, got %v", testCase.inputEventNID, testCase.wantEvent, gotEvent)
		}
	}

}
