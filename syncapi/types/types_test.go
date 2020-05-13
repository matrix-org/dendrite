package types

import "testing"

func TestNewSyncTokenFromString(t *testing.T) {
	shouldPass := map[string]syncToken{
		"s4_0": NewStreamToken(4, 0).syncToken,
		"s3_1": NewStreamToken(3, 1).syncToken,
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
		result, err := newSyncTokenFromString(test)
		if err != nil {
			t.Error(err)
		}
		if result.String() != expected.String() {
			t.Errorf("%s expected %v but got %v", test, expected.String(), result.String())
		}
	}

	for _, test := range shouldFail {
		if _, err := newSyncTokenFromString(test); err == nil {
			t.Errorf("input '%v' should have errored but didn't", test)
		}
	}
}
