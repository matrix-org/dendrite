package gomatrixserverlib

import (
	"testing"
)

const (
	sha1OfEventID1A = "\xe5\x89,\xa2\x1cF<&\xf3\rf}\xde\xa5\xef;\xddK\xaaS"
	sha1OfEventID2A = "\xa4\xe4\x10\x1b}\x1a\xf9`\x94\x10\xa3\x84+\xae\x06\x8d\x16A\xfc>"
	sha1OfEventID3B = "\xca\xe8\xde\xb6\xa3\xb6\xee\x01\xc4\xbc\xd0/\x1b\x1c2\x0c\xd3\xa4\xe9\xcb"
)

func TestConflictEventSorter(t *testing.T) {
	input := []Event{
		{fields: eventFields{Depth: 1, EventID: "@1:a"}},
		{fields: eventFields{Depth: 2, EventID: "@2:a"}},
		{fields: eventFields{Depth: 2, EventID: "@3:b"}},
	}
	got := sortConflictedEventsByDepthAndSHA1(input)
	want := []conflictedEvent{
		{depth: 1, event: &input[0]},
		{depth: 2, event: &input[2]},
		{depth: 2, event: &input[1]},
	}
	copy(want[0].eventIDSHA1[:], sha1OfEventID1A)
	copy(want[1].eventIDSHA1[:], sha1OfEventID3B)
	copy(want[2].eventIDSHA1[:], sha1OfEventID2A)
	if len(want) != len(got) {
		t.Fatalf("Different length: wanted %d, got %d", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("Different element at index %d: wanted %#v got %#v", i, want[i], got[i])
		}
	}
}
