// Copyright 2017 Vector Creations Ltd
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

package jsonerror

import (
	"fmt"

	"github.com/matrix-org/util"
)

// MatrixError represents the "standard error response" in Matrix.
// http://matrix.org/docs/spec/client_server/r0.2.0.html#api-standards
type MatrixError struct {
	ErrCode string `json:"errcode"`
	Err     string `json:"error"`
}

func (e *MatrixError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrCode, e.Err)
}

// InternalServerError returns a 500 Internal Server Error in a matrix-compliant
// format.
func InternalServerError() util.JSONResponse {
	return util.JSONResponse{
		Code: 500,
		JSON: Unknown("Internal Server Error"),
	}
}

// Unknown is an unexpected error
func Unknown(msg string) *MatrixError {
	return &MatrixError{"M_UNKNOWN", msg}
}

// Forbidden is an error when the client tries to access a resource
// they are not allowed to access.
func Forbidden(msg string) *MatrixError {
	return &MatrixError{"M_FORBIDDEN", msg}
}

// BadJSON is an error when the client supplies malformed JSON.
func BadJSON(msg string) *MatrixError {
	return &MatrixError{"M_BAD_JSON", msg}
}

// NotJSON is an error when the client supplies something that is not JSON
// to a JSON endpoint.
func NotJSON(msg string) *MatrixError {
	return &MatrixError{"M_NOT_JSON", msg}
}

// NotFound is an error when the client tries to access an unknown resource.
func NotFound(msg string) *MatrixError {
	return &MatrixError{"M_NOT_FOUND", msg}
}

// MissingToken is an error when the client tries to access a resource which
// requires authentication without supplying credentials.
func MissingToken(msg string) *MatrixError {
	return &MatrixError{"M_MISSING_TOKEN", msg}
}

// UnknownToken is an error when the client tries to access a resource which
// requires authentication and supplies a valid, but out-of-date token.
func UnknownToken(msg string) *MatrixError {
	return &MatrixError{"M_UNKNOWN_TOKEN", msg}
}

// WeakPassword is an error which is returned when the client tries to register
// using a weak password. http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
func WeakPassword(msg string) *MatrixError {
	return &MatrixError{"M_WEAK_PASSWORD", msg}
}

// LimitExceededError is a rate-limiting error.
type LimitExceededError struct {
	MatrixError
	RetryAfterMS int64 `json:"retry_after_ms,omitempty"`
}

// LimitExceeded is an error when the client tries to send events too quickly.
func LimitExceeded(msg string, retryAfterMS int64) *LimitExceededError {
	return &LimitExceededError{
		MatrixError:  MatrixError{"M_LIMIT_EXCEEDED", msg},
		RetryAfterMS: retryAfterMS,
	}
}
