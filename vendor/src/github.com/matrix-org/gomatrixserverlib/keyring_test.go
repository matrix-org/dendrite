package gomatrixserverlib

import (
	"encoding/json"
	"testing"
)

var privateKeySeed1 = `QJvXAPj0D9MUb1exkD8pIWmCvT1xajlsB8jRYz/G5HE`
var privateKeySeed2 = `AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`

// testKeys taken from a copy of synapse.
var testKeys = `{
	"old_verify_keys": {
		"ed25519:old": {
			"expired_ts": 929059200,
			"key": "O2onvM62pC1io6jQKm8Nc2UyFXcd4kOmOsBIoYtZ2ik"
		}
	},
	"server_name": "localhost:8800",
	"signatures": {
		"localhost:8800": {
			"ed25519:a_Obwu": "xkr4Z49ODoQnRi//ePfXlt8Q68vzd+DkzBNCt60NcwnLjNREx0qVQrw1iTFSoxkgGtz30NDkmyffDrCrmX5KBw"
		}
	},
	"tls_fingerprints": [
		{
			"sha256": "I2ohBnqpb5m3HldWFwyA10WdjqDksukiKVUdZ690WzM"
		}
	],
	"valid_until_ts": 1493142432964,
	"verify_keys": {
		"ed25519:a_Obwu": {
			"key": "2UwTWD4+tgTgENV7znGGNqhAOGY+BW1mRAnC6W6FBQg"
		}
	}
}`

type testKeyDatabase struct{}

func (db *testKeyDatabase) FetchKeys(requests map[PublicKeyRequest]Timestamp) (map[PublicKeyRequest]ServerKeys, error) {
	results := map[PublicKeyRequest]ServerKeys{}
	var keys ServerKeys
	if err := json.Unmarshal([]byte(testKeys), &keys); err != nil {
		return nil, err
	}

	req1 := PublicKeyRequest{"localhost:8800", "ed25519:old"}
	req2 := PublicKeyRequest{"localhost:8800", "ed25519:a_Obwu"}

	for req := range requests {
		if req == req1 || req == req2 {
			results[req] = keys
		}
	}
	return results, nil
}

func (db *testKeyDatabase) StoreKeys(requests map[PublicKeyRequest]ServerKeys) error {
	return nil
}

func TestVerifyJSONsSuccess(t *testing.T) {
	// Check that trying to verify the server key JSON works.
	k := KeyRing{nil, &testKeyDatabase{}}
	results, err := k.VerifyJSONs([]VerifyJSONRequest{{
		ServerName: "localhost:8800",
		Message:    []byte(testKeys),
		AtTS:       1493142432964,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Error != nil {
		t.Fatalf("VerifyJSON(): Wanted [{Error: nil}] got %#v", results)
	}
}

func TestVerifyJSONsUnknownServerFails(t *testing.T) {
	// Check that trying to verify JSON for an unknown server fails.
	k := KeyRing{nil, &testKeyDatabase{}}
	results, err := k.VerifyJSONs([]VerifyJSONRequest{{
		ServerName: "unknown:8800",
		Message:    []byte(testKeys),
		AtTS:       1493142432964,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Error == nil {
		t.Fatalf("VerifyJSON(): Wanted [{Error: <some error>}] got %#v", results)
	}
}

func TestVerifyJSONsDistantFutureFails(t *testing.T) {
	// Check that trying to verify JSON from the distant future fails.
	distantFuture := Timestamp(2000000000000)
	k := KeyRing{nil, &testKeyDatabase{}}
	results, err := k.VerifyJSONs([]VerifyJSONRequest{{
		ServerName: "unknown:8800",
		Message:    []byte(testKeys),
		AtTS:       distantFuture,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Error == nil {
		t.Fatalf("VerifyJSON(): Wanted [{Error: <some error>}] got %#v", results)
	}
}

func TestVerifyJSONsFetcherError(t *testing.T) {
	// Check that if the database errors then the attempt to verify JSON fails.
	k := KeyRing{nil, &erroringKeyDatabase{}}
	results, err := k.VerifyJSONs([]VerifyJSONRequest{{
		ServerName: "localhost:8800",
		Message:    []byte(testKeys),
		AtTS:       1493142432964,
	}})
	if err != error(&testErrorFetch) || results != nil {
		t.Fatalf("VerifyJSONs(): Wanted (nil, <some error>) got (%#v, %q)", results, err)
	}
}

type erroringKeyDatabase struct{}

type erroringKeyDatabaseError int

func (e *erroringKeyDatabaseError) Error() string { return "An error with the key database" }

var testErrorFetch = erroringKeyDatabaseError(1)
var testErrorStore = erroringKeyDatabaseError(2)

func (e *erroringKeyDatabase) FetchKeys(requests map[PublicKeyRequest]Timestamp) (map[PublicKeyRequest]ServerKeys, error) {
	return nil, &testErrorFetch
}

func (e *erroringKeyDatabase) StoreKeys(keys map[PublicKeyRequest]ServerKeys) error {
	return &testErrorStore
}
