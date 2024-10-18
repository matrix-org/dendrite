// Copyright 2024 New Vector Ltd.
// Copyright 2017 Vector Creations Ltd
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package consumers

import (
	"testing"
)

func TestCombineNoOp(t *testing.T) {
	inputAdd1 := []string{"a", "b", "c"}
	inputDel1 := []string{"a", "b", "d"}
	inputAdd2 := []string{"a", "d", "e"}
	inputDel2 := []string{"a", "c", "e", "e"}

	gotAdd, gotDel := combineDeltas(inputAdd1, inputDel1, inputAdd2, inputDel2)

	if len(gotAdd) != 0 {
		t.Errorf("wanted combined adds to be an empty list, got %#v", gotAdd)
	}

	if len(gotDel) != 0 {
		t.Errorf("wanted combined removes to be an empty list, got %#v", gotDel)
	}
}

func TestCombineDedup(t *testing.T) {
	inputAdd1 := []string{"a", "a"}
	inputDel1 := []string{"b", "b"}
	inputAdd2 := []string{"a", "a"}
	inputDel2 := []string{"b", "b"}

	gotAdd, gotDel := combineDeltas(inputAdd1, inputDel1, inputAdd2, inputDel2)

	if len(gotAdd) != 1 || gotAdd[0] != "a" {
		t.Errorf("wanted combined adds to be %#v, got %#v", []string{"a"}, gotAdd)
	}

	if len(gotDel) != 1 || gotDel[0] != "b" {
		t.Errorf("wanted combined removes to be %#v, got %#v", []string{"b"}, gotDel)
	}
}
