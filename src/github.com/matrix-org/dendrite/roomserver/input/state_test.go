package input

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"testing"
)

func TestFindDuplicateStateKeys(t *testing.T) {
	testCases := []struct {
		Input []types.StateEntry
		Want  []types.StateEntry
	}{{
		Input: []types.StateEntry{
			{types.StateKeyTuple{1, 1}, 1},
			{types.StateKeyTuple{1, 1}, 2},
			{types.StateKeyTuple{2, 2}, 3},
		},
		Want: []types.StateEntry{
			{types.StateKeyTuple{1, 1}, 1},
			{types.StateKeyTuple{1, 1}, 2},
		},
	}, {
		Input: []types.StateEntry{
			{types.StateKeyTuple{1, 1}, 1},
			{types.StateKeyTuple{1, 2}, 2},
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
