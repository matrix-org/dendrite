// Copyright 2024 New Vector Ltd.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package test

import "sort"

// UnsortedStringSliceEqual returns true if the slices have same length & elements.
// Does not modify the given slice.
func UnsortedStringSliceEqual(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}

	a, b := first[:], second[:]
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
