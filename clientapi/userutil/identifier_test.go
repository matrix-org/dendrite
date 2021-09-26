// Copyright 2022 The Matrix.org Foundation C.I.C.
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

package userutil

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAnyIdentifierJSON(t *testing.T) {
	tsts := []struct {
		Name string
		JSON string
		Want Identifier
	}{
		{Name: "empty", JSON: `{}`},
		{Name: "user", JSON: `{"type":"m.id.user","user":"auser"}`, Want: &UserIdentifier{UserID: "auser"}},
		{Name: "thirdparty", JSON: `{"type":"m.id.thirdparty","medium":"email","address":"auser@example.com"}`, Want: &ThirdPartyIdentifier{Medium: "email", Address: "auser@example.com"}},
		{Name: "phone", JSON: `{"type":"m.id.phone","country":"GB","phone":"123456789"}`, Want: &PhoneIdentifier{Country: "GB", PhoneNumber: "123456789"}},
		// This test is a little fragile since it compares the output of json.Marshal.
		{Name: "unknown", JSON: `{"type":"other"}`, Want: &UnknownIdentifier{Type: "other", RawMessage: json.RawMessage(`{"type":"other"}`)}},
	}
	for _, tst := range tsts {
		t.Run("Unmarshal/"+tst.Name, func(t *testing.T) {
			var got AnyIdentifier
			if err := json.Unmarshal([]byte(tst.JSON), &got); err != nil {
				if tst.Want == nil {
					return
				}
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(got.Identifier, tst.Want) {
				t.Errorf("got %+v, want %+v", got.Identifier, tst.Want)
			}
		})

		if tst.Want == nil {
			continue
		}
		t.Run("Marshal/"+tst.Name, func(t *testing.T) {
			id := AnyIdentifier{Identifier: tst.Want}
			bs, err := json.Marshal(id)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			t.Logf("Marshalled JSON: %q", string(bs))

			var got AnyIdentifier
			if err := json.Unmarshal(bs, &got); err != nil {
				if tst.Want == nil {
					return
				}
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if !reflect.DeepEqual(got.Identifier, tst.Want) {
				t.Errorf("got %+v, want %+v", got.Identifier, tst.Want)
			}
		})
	}
}
