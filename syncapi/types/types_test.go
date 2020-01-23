package types

import "testing"

func TestNewPaginationTokenFromString(t *testing.T) {
	shouldPass := map[string]PaginationToken{
		"2": PaginationToken{
			Type:        PaginationTokenTypeStream,
			PDUPosition: 2,
		},
		"s4": PaginationToken{
			Type:        PaginationTokenTypeStream,
			PDUPosition: 4,
		},
		"s3_1": PaginationToken{
			Type:              PaginationTokenTypeStream,
			PDUPosition:       3,
			EDUTypingPosition: 1,
		},
		"t3_1_4": PaginationToken{
			Type:              PaginationTokenTypeTopology,
			PDUPosition:       3,
			EDUTypingPosition: 1,
		},
	}

	shouldFail := []string{
		"",
		"s_1",
		"s_",
		"a3_4",
		"b",
		"b-1",
		"-4",
	}

	for test, expected := range shouldPass {
		result, err := NewPaginationTokenFromString(test)
		if err != nil {
			t.Error(err)
		}
		if *result != expected {
			t.Errorf("expected %v but got %v", expected.String(), result.String())
		}
	}

	for _, test := range shouldFail {
		if _, err := NewPaginationTokenFromString(test); err == nil {
			t.Errorf("input '%v' should have errored but didn't", test)
		}
	}
}
