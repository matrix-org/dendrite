package types

import (
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
