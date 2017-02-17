package input

import (
	"github.com/matrix-org/dendrite/roomserver/types"
	"testing"
)

type sortBytes []byte

func (s sortBytes) Len() int           { return len(s) }
func (s sortBytes) Less(i, j int) bool { return s[i] < s[j] }
func (s sortBytes) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func TestUnique(t *testing.T) {
	testCases := []struct {
		Input string
		Want  string
	}{
		{"", ""},
		{"abc", "abc"},
		{"aaabbbccc", "abc"},
	}

	for _, test := range testCases {
		input := []byte(test.Input)
		want := string(test.Want)
		got := string(input[:unique(sortBytes(input))])
		if got != want {
			t.Fatal("Wanted ", want, " got ", got)
		}
	}
}

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
