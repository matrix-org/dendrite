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

package jsonerror

import (
	"encoding/json"
	"testing"
)

func TestLimitExceeded(t *testing.T) {
	e := LimitExceeded("too fast", 5000)
	jsonBytes, err := json.Marshal(&e)
	if err != nil {
		t.Fatalf("TestLimitExceeded: Failed to marshal LimitExceeded error. %s", err.Error())
	}
	want := `{"errcode":"M_LIMIT_EXCEEDED","error":"too fast","retry_after_ms":5000}`
	if string(jsonBytes) != want {
		t.Errorf("TestLimitExceeded: want %s, got %s", want, string(jsonBytes))
	}
}

func TestForbidden(t *testing.T) {
	e := Forbidden("you shall not pass")
	jsonBytes, err := json.Marshal(&e)
	if err != nil {
		t.Fatalf("TestForbidden: Failed to marshal Forbidden error. %s", err.Error())
	}
	want := `{"errcode":"M_FORBIDDEN","error":"you shall not pass"}`
	if string(jsonBytes) != want {
		t.Errorf("TestForbidden: want %s, got %s", want, string(jsonBytes))
	}
}
