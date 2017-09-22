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

func TestMarshalHex(t *testing.T) {
	input := HexString("this\xffis\xffa\xfftest")
	want := `"74686973ff6973ff61ff74657374"`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(HexString(%q)): wanted %q got %q", string(input), want, string(got))
	}
}

func TestUnmarshalHex(t *testing.T) {
	input := []byte(`"74686973ff6973ff61ff74657374"`)
	want := "this\xffis\xffa\xfftest"
	var got HexString
	err := json.Unmarshal(input, &got)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Unmarshal(%q): wanted %q got %q", string(input), want, string(got))
	}
}

func TestMarshalHexStruct(t *testing.T) {
	input := struct{ Value HexString }{HexString("this\xffis\xffa\xfftest")}
	want := `{"Value":"74686973ff6973ff61ff74657374"}`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalHexMap(t *testing.T) {
	input := map[string]HexString{"Value": HexString("this\xffis\xffa\xfftest")}
	want := `{"Value":"74686973ff6973ff61ff74657374"}`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalHexSlice(t *testing.T) {
	input := []HexString{HexString("this\xffis\xffa\xfftest")}
	want := `["74686973ff6973ff61ff74657374"]`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}
