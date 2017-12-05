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

	"gopkg.in/yaml.v2"
)

func TestMarshalBase64(t *testing.T) {
	input := Base64String("this\xffis\xffa\xfftest")
	want := `"dGhpc/9pc/9h/3Rlc3Q"`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(Base64String(%q)): wanted %q got %q", string(input), want, string(got))
	}
}

func TestUnmarshalBase64(t *testing.T) {
	input := []byte(`"dGhpc/9pc/9h/3Rlc3Q"`)
	want := "this\xffis\xffa\xfftest"
	var got Base64String
	err := json.Unmarshal(input, &got)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Unmarshal(%q): wanted %q got %q", string(input), want, string(got))
	}
}

func TestUnmarshalUrlSafeBase64(t *testing.T) {
	input := []byte(`"dGhpc_9pc_9h_3Rlc3Q"`)
	want := "this\xffis\xffa\xfftest"
	var got Base64String
	err := json.Unmarshal(input, &got)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Unmarshal(%q): wanted %q got %q", string(input), want, string(got))
	}
}

func TestMarshalBase64Struct(t *testing.T) {
	input := struct{ Value Base64String }{Base64String("this\xffis\xffa\xfftest")}
	want := `{"Value":"dGhpc/9pc/9h/3Rlc3Q"}`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalBase64Map(t *testing.T) {
	input := map[string]Base64String{"Value": Base64String("this\xffis\xffa\xfftest")}
	want := `{"Value":"dGhpc/9pc/9h/3Rlc3Q"}`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalBase64Slice(t *testing.T) {
	input := []Base64String{Base64String("this\xffis\xffa\xfftest")}
	want := `["dGhpc/9pc/9h/3Rlc3Q"]`
	got, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalYAMLBase64(t *testing.T) {
	input := Base64String("this\xffis\xffa\xfftest")
	want := "dGhpc/9pc/9h/3Rlc3Q\n"
	got, err := yaml.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("yaml.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestMarshalYAMLBase64Struct(t *testing.T) {
	input := struct{ Value Base64String }{Base64String("this\xffis\xffa\xfftest")}
	want := "value: dGhpc/9pc/9h/3Rlc3Q\n"
	got, err := yaml.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("yaml.Marshal(%v): wanted %q got %q", input, want, string(got))
	}
}

func TestUnmarshalYAMLBase64(t *testing.T) {
	input := []byte("dGhpc/9pc/9h/3Rlc3Q")
	want := Base64String("this\xffis\xffa\xfftest")
	var got Base64String
	err := yaml.Unmarshal(input, &got)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("yaml.Unmarshal(%q): wanted %q got %q", string(input), want, string(got))
	}
}

func TestUnmarshalYAMLBase64Struct(t *testing.T) {
	// var u yaml.Unmarshaler
	u := Base64String("this\xffis\xffa\xfftest")

	input := []byte(`value: dGhpc/9pc/9h/3Rlc3Q`)
	want := struct{ Value Base64String }{u}
	result := struct {
		Value Base64String `yaml:"value"`
	}{}
	err := yaml.Unmarshal(input, &result)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Value) != string(want.Value) {
		t.Fatalf("yaml.Unmarshal(%v): wanted %q got %q", input, want, result)
	}
}
