// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
