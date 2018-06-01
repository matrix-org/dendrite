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
	"time"
)

var (
	// If any of these options are missing, validation should fail
	invalidMissings   = []string{"ServerPrivateKey", "UserID"}
	invalidKeyTokenOp = TokenOptions{
		ServerPrivateKey: []byte("notASecretKey"),
		UserID:           "aRandomUserID",
	}
	invalidUserTokenOp = TokenOptions{
		ServerPrivateKey: []byte("aSecretKey"),
		UserID:           "notTheSameUserID",
	}
)

func expireZeroValidTokenOp() TokenOptions {
	op := validTokenOp
	op.Duration = 0
	return op
}

func TestExpiredLoginToken(t *testing.T) {
	fakeToken, err := GenerateLoginToken(expireZeroValidTokenOp())
	// token uses 1 second precision
	time.Sleep(time.Second)
	res := ValidateToken(validTokenOp, fakeToken)
	if res == nil {
		t.Error("Token validation should fail for expired token")
	}
}

func TestValidateToken(t *testing.T) {
	fakeToken, err := GenerateLoginToken(validTokenOp)
	if err != nil {
		t.Errorf("Token generation failed for valid TokenOptions with err: %s", err.Error())
	}

	// Test validation
	res := ValidateToken(validTokenOp, fakeToken)
	if res != nil {
		t.Error("Token validation failed with response: ", res)
	}

	// Test validation fails for invalid TokenOp
	for _, invalidMissing := range invalidMissings {
		res = ValidateToken(invalidTokenOps[invalidMissing], fakeToken)
		if res == nil {
			t.Errorf("Token validation should fail for TokenOptions with missing %s", invalidMissing)
		}
	}

	for _, invalid := range []TokenOptions{invalidKeyTokenOp, invalidUserTokenOp} {
		res = ValidateToken(invalid, fakeToken)
		if res == nil {
			t.Errorf("Token validation should fail for invalid TokenOptions: ", invalid)
		}
	}
}

func TestGetUserFromToken(t *testing.T) {
	fakeToken, err := GenerateLoginToken(validTokenOp)
	if err != nil {
		t.Errorf("Token generation failed for valid TokenOptions with err: %s", err.Error())
	}

	// Test validation
	name, err := GetUserFromToken(fakeToken)
	if err != nil {
		t.Error("Failed to get userID from Token: ", err)
	}

	if name != validTokenOp.UserID {
		t.Error("UserID from Token doesn't match, got: ", name, " expected: ", validTokenOp.UserID)
	}
}
