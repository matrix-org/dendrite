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

import "testing"

// If any of these options are missing, validation should fail
var invalidMissings = []string{"ServerPrivateKey", "UserID"}

func TestValidateToken(t *testing.T) {
	fakeToken, err := GenerateLoginToken(validTokenOp)
	if err != nil {
		t.Errorf("Token generation failed for valid TokenOptions with err: %s", err.Error())
	}

	// Test validation
	res := ValidateToken(validTokenOp, fakeToken)
	if res != nil {
		t.Error("Token validation failed with response: ", *res)
	}

	// Test validation fails for invalid TokenOp
	for _, invalidMissing := range invalidMissings {
		res = ValidateToken(invalidTokenOps[invalidMissing], fakeToken)
		if res == nil {
			t.Errorf("Token validation should fail for TokenOptions with missing %s", invalidMissing)
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
