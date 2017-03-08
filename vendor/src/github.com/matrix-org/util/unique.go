package util

import (
	"fmt"
	"sort"
)

// Unique removes duplicate items from a sorted list in place.
// Takes the same interface as sort.Sort
// Returns the length of the data without duplicates
// Uses the last occurrence of a duplicate.
// O(n).
func Unique(data sort.Interface) int {
	if !sort.IsSorted(data) {
		panic(fmt.Errorf("util: the input to Unique() must be sorted"))
	}

	if data.Len() == 0 {
		return 0
	}
	length := data.Len()
	// j is the next index to output an element to.
	j := 0
	for i := 1; i < length; i++ {
		// If the previous element is less than this element then they are
		// not equal. Otherwise they must be equal because the list is sorted.
		// If they are equal then we move onto the next element.
		if data.Less(i-1, i) {
			// "Write" the previous element to the output position by swapping
			// the elements.
			// Note that if the list has no duplicates then i-1 == j so the
			// swap does nothing. (This assumes that data.Swap(a,b) nops if a==b)
			data.Swap(i-1, j)
			// Advance to the next output position in the list.
			j++
		}
	}
	// Output the last element.
	data.Swap(length-1, j)
	return j + 1
}

// SortAndUnique sorts a list and removes duplicate entries in place.
// Takes the same interface as sort.Sort
// Returns the length of the data without duplicates
// Uses the last occurrence of a duplicate.
// O(nlog(n))
func SortAndUnique(data sort.Interface) int {
	sort.Sort(data)
	return Unique(data)
}

// UniqueStrings turns a list of strings into a sorted list of unique strings.
// O(nlog(n))
func UniqueStrings(strings []string) []string {
	return strings[:SortAndUnique(sort.StringSlice(strings))]
}
