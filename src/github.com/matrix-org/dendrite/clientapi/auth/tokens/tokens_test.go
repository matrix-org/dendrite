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

package tokens

import (
	"testing"
)

var (
	validTokenOp = TokenOptions{
		ServerPrivateKey: []byte("aSecretKey"),
		ServerName:       "aRandomServerName",
		UserID:           "aRandomUserID",
	}
	invalidTokenOps = map[string]TokenOptions{
		"ServerPrivateKey": {
			ServerName: "aRandomServerName",
			UserID:     "aRandomUserID",
		},
		"ServerName": {
			ServerPrivateKey: []byte("aSecretKey"),
			UserID:           "aRandomUserID",
		},
		"UserID": {
			ServerPrivateKey: []byte("aSecretKey"),
			ServerName:       "aRandomServerName",
		},
	}
)

func TestGenerateLoginToken(t *testing.T) {
	// Test valid
	_, err := GenerateLoginToken(validTokenOp)
	if err != nil {
		t.Errorf("Token generation failed for valid TokenOptions with err: %s", err.Error())
	}

	// Test invalids
	for missing, invalidTokenOp := range invalidTokenOps {
		_, err := GenerateLoginToken(invalidTokenOp)
		if err == nil {
			t.Errorf("Token generation should fail for TokenOptions with missing %s", missing)
		}
	}
}

func serializationTestError(err error) string {
	return "Token Serialization test failed with err: " + err.Error()
}

func TestSerialization(t *testing.T) {
	fakeToken, err := GenerateLoginToken(validTokenOp)
	if err != nil {
		t.Errorf(serializationTestError(err))
	}

	fakeMacaroon, err := deSerializeMacaroon(fakeToken)
	if err != nil {
		t.Errorf(serializationTestError(err))
	}

	sameFakeToken, err := serializeMacaroon(fakeMacaroon)
	if err != nil {
		t.Errorf(serializationTestError(err))
	}

	if sameFakeToken != fakeToken {
		t.Errorf("Token Serialization mismatch")
	}
}
