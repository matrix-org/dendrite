/* Copyright 2016-2017 Vector Creations Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"encoding/json"
	"testing"
)

func TestLevelJSONValueValid(t *testing.T) {
	var values []levelJSONValue
	input := `[0,"1",2.0]`
	if err := json.Unmarshal([]byte(input), &values); err != nil {
		t.Fatal("Unexpected error unmarshalling ", input, ": ", err)
	}
	for i, got := range values {
		want := i
		if !got.exists {
			t.Fatalf("Wanted entry %d to exist", want)
		}
		if int64(want) != got.value {
			t.Fatalf("Wanted %d got %q", want, got.value)
		}
	}
}

func TestLevelJSONValueInvalid(t *testing.T) {
	var values []levelJSONValue
	inputs := []string{
		`[{}]`, `[[]]`, `["not a number"]`, `["0.0"]`,
	}

	for _, input := range inputs {
		if err := json.Unmarshal([]byte(input), &values); err == nil {
			t.Fatalf("Unexpected success when unmarshalling %q", input)
		}
	}
}
