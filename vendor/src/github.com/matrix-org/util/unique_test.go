package util

import (
	"sort"
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
		got := string(input[:Unique(sortBytes(input))])
		if got != want {
			t.Fatal("Wanted ", want, " got ", got)
		}
	}
}

type sortByFirstByte []string

func (s sortByFirstByte) Len() int           { return len(s) }
func (s sortByFirstByte) Less(i, j int) bool { return s[i][0] < s[j][0] }
func (s sortByFirstByte) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func TestUniquePicksLastDuplicate(t *testing.T) {
	input := []string{
		"aardvark",
		"avacado",
		"cat",
		"cucumber",
	}
	want := []string{
		"avacado",
		"cucumber",
	}
	got := input[:Unique(sortByFirstByte(input))]

	if len(want) != len(got) {
		t.Errorf("Wanted %#v got %#v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("Wanted %#v got %#v", want, got)
		}
	}
}

func TestUniquePanicsIfNotSorted(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Unique() to panic on unsorted input but it didn't")
		}
	}()
	Unique(sort.StringSlice{"out", "of", "order"})
}

func TestUniqueStrings(t *testing.T) {
	input := []string{
		"badger", "badger", "badger", "badger",
		"badger", "badger", "badger", "badger",
		"badger", "badger", "badger", "badger",
		"mushroom", "mushroom",
		"badger", "badger", "badger", "badger",
		"badger", "badger", "badger", "badger",
		"badger", "badger", "badger", "badger",
		"snake", "snake",
	}

	want := []string{"badger", "mushroom", "snake"}

	got := UniqueStrings(input)

	if len(want) != len(got) {
		t.Errorf("Wanted %#v got %#v", want, got)
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("Wanted %#v got %#v", want, got)
		}
	}
}
