package types

import (
	"reflect"
	"testing"
)

func TestNewSyncTokenWithLogs(t *testing.T) {
	tests := map[string]*StreamingToken{
		"s4_0": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs:      make(map[string]*LogPosition),
		},
		"s4_0.dl-0-123": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs: map[string]*LogPosition{
				"dl": &LogPosition{
					Partition: 0,
					Offset:    123,
				},
			},
		},
		"s4_0.ab-1-14419482332.dl-0-123": &StreamingToken{
			syncToken: syncToken{Type: "s", Positions: []StreamPosition{4, 0}},
			logs: map[string]*LogPosition{
				"ab": &LogPosition{
					Partition: 1,
					Offset:    14419482332,
				},
				"dl": &LogPosition{
					Partition: 0,
					Offset:    123,
				},
			},
		},
	}
	for tok, want := range tests {
		got, err := NewStreamTokenFromString(tok)
		if err != nil {
			if want == nil {
				continue // error expected
			}
			t.Errorf("%s errored: %s", tok, err)
			continue
		}
		if !reflect.DeepEqual(got, *want) {
			t.Errorf("%s mismatch: got %v want %v", tok, got, want)
		}
		gotStr := got.String()
		if gotStr != tok {
			t.Errorf("%s reserialisation mismatch: got %s want %s", tok, gotStr, tok)
		}
	}
}

func TestNewSyncTokenFromString(t *testing.T) {
	shouldPass := map[string]syncToken{
		"s4_0": NewStreamToken(4, 0, nil).syncToken,
		"s3_1": NewStreamToken(3, 1, nil).syncToken,
		"t3_1": NewTopologyToken(3, 1).syncToken,
	}

	shouldFail := []string{
		"",
		"s_1",
		"s_",
		"a3_4",
		"b",
		"b-1",
		"-4",
		"2",
	}

	for test, expected := range shouldPass {
		result, _, err := newSyncTokenFromString(test)
		if err != nil {
			t.Error(err)
		}
		if result.String() != expected.String() {
			t.Errorf("%s expected %v but got %v", test, expected.String(), result.String())
		}
	}

	for _, test := range shouldFail {
		if _, _, err := newSyncTokenFromString(test); err == nil {
			t.Errorf("input '%v' should have errored but didn't", test)
		}
	}
}
