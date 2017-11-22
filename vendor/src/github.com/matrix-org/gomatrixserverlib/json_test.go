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
	"testing"
)

func testSortJSON(t *testing.T, input, want string) {
	got := SortJSON([]byte(input), nil)

	// Squash out the whitespace before comparing the JSON in case SortJSON had inserted whitespace.
	if string(CompactJSON(got, nil)) != want {
		t.Errorf("SortJSON(%q): want %q got %q", input, want, got)
	}
}

func TestSortJSON(t *testing.T) {
	testSortJSON(t, `[{"b":"two","a":1}]`, `[{"a":1,"b":"two"}]`)
	testSortJSON(t, `{"B":{"4":4,"3":3},"A":{"1":1,"2":2}}`,
		`{"A":{"1":1,"2":2},"B":{"3":3,"4":4}}`)
	testSortJSON(t, `[true,false,null]`, `[true,false,null]`)
	testSortJSON(t, `[9007199254740991]`, `[9007199254740991]`)
	testSortJSON(t, "\t\n[9007199254740991]", `[9007199254740991]`)
}

func testCompactJSON(t *testing.T, input, want string) {
	got := string(CompactJSON([]byte(input), nil))
	if got != want {
		t.Errorf("CompactJSON(%q): want %q got %q", input, want, got)
	}
}

func TestCompactJSON(t *testing.T) {
	testCompactJSON(t, "{ }", "{}")

	input := `["\u0000\u0001\u0002\u0003\u0004\u0005\u0006\u0007"]`
	want := input
	testCompactJSON(t, input, want)

	input = `["\u0008\u0009\u000A\u000B\u000C\u000D\u000E\u000F"]`
	want = `["\b\t\n\u000B\f\r\u000E\u000F"]`
	testCompactJSON(t, input, want)

	input = `["\u0010\u0011\u0012\u0013\u0014\u0015\u0016\u0017"]`
	want = input
	testCompactJSON(t, input, want)

	input = `["\u0018\u0019\u001A\u001B\u001C\u001D\u001E\u001F"]`
	want = input
	testCompactJSON(t, input, want)

	testCompactJSON(t, `["\u0061\u005C\u0042\u0022"]`, `["a\\B\""]`)
	testCompactJSON(t, `["\u0120"]`, "[\"\u0120\"]")
	testCompactJSON(t, `["\u0FFF"]`, "[\"\u0FFF\"]")
	testCompactJSON(t, `["\u1820"]`, "[\"\u1820\"]")
	testCompactJSON(t, `["\uFFFF"]`, "[\"\uFFFF\"]")
	testCompactJSON(t, `["\uD842\uDC20"]`, "[\"\U00020820\"]")
	testCompactJSON(t, `["\uDBFF\uDFFF"]`, "[\"\U0010FFFF\"]")

	testCompactJSON(t, `["\"\\\/"]`, `["\"\\/"]`)
}

func testReadHex(t *testing.T, input string, want uint32) {
	got := readHexDigits([]byte(input))
	if want != got {
		t.Errorf("readHexDigits(%q): want 0x%x got 0x%x", input, want, got)
	}
}

func TestReadHex(t *testing.T) {
	testReadHex(t, "0123", 0x0123)
	testReadHex(t, "4567", 0x4567)
	testReadHex(t, "89AB", 0x89AB)
	testReadHex(t, "CDEF", 0xCDEF)
	testReadHex(t, "89ab", 0x89AB)
	testReadHex(t, "cdef", 0xCDEF)
}
