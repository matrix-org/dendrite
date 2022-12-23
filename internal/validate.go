// Copyright 2022 The Matrix.org Foundation C.I.C.
//
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

package internal

import (
	"fmt"
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

const minPasswordLength = 8 // http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based

const maxPasswordLength = 512 // https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161

// ValidatePassword returns an error response if the password is invalid
func ValidatePassword(password string) *util.JSONResponse {
	// https://github.com/matrix-org/synapse/blob/v0.20.0/synapse/rest/client/v2_alpha/register.py#L161
	if len(password) > maxPasswordLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.BadJSON(fmt.Sprintf("password too long: max %d characters", maxPasswordLength)),
		}
	} else if len(password) > 0 && len(password) < minPasswordLength {
		return &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.WeakPassword(fmt.Sprintf("password too weak: min %d chars", minPasswordLength)),
		}
	}
	return nil
}
