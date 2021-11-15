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
	"net/http"

	"github.com/matrix-org/gomatrixserverlib"
	"github.com/matrix-org/util"
)

// MatrixError represents the "standard error response" in Matrix.
// http://matrix.org/docs/spec/client_server/r0.2.0.html#api-standards
type MatrixError struct {
	ErrCode string `json:"errcode"`
	Err     string `json:"error"`
}

func (e MatrixError) Error() string {
	return fmt.Sprintf("%s: %s", e.ErrCode, e.Err)
}

// InternalServerError returns a 500 Internal Server Error in a matrix-compliant
// format.
func InternalServerError() util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusInternalServerError,
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

// MissingArgument is an error when the client tries to access a resource
// without providing an argument that is required.
func MissingArgument(msg string) *MatrixError {
	return &MatrixError{"M_MISSING_ARGUMENT", msg}
}

// InvalidArgumentValue is an error when the client tries to provide an
// invalid value for a valid argument
func InvalidArgumentValue(msg string) *MatrixError {
	return &MatrixError{"M_INVALID_ARGUMENT_VALUE", msg}
}

// MissingToken is an error when the client tries to access a resource which
// requires authentication without supplying credentials.
func MissingToken(msg string) *MatrixError {
	return &MatrixError{"M_MISSING_TOKEN", msg}
}

// UnknownToken is an error when the client tries to access a resource which
// requires authentication and supplies an unrecognised token
func UnknownToken(msg string) *MatrixError {
	return &MatrixError{"M_UNKNOWN_TOKEN", msg}
}

// WeakPassword is an error which is returned when the client tries to register
// using a weak password. http://matrix.org/docs/spec/client_server/r0.2.0.html#password-based
func WeakPassword(msg string) *MatrixError {
	return &MatrixError{"M_WEAK_PASSWORD", msg}
}

// InvalidUsername is an error returned when the client tries to register an
// invalid username
func InvalidUsername(msg string) *MatrixError {
	return &MatrixError{"M_INVALID_USERNAME", msg}
}

// UserInUse is an error returned when the client tries to register an
// username that already exists
func UserInUse(msg string) *MatrixError {
	return &MatrixError{"M_USER_IN_USE", msg}
}

// RoomInUse is an error returned when the client tries to make a room
// that already exists
func RoomInUse(msg string) *MatrixError {
	return &MatrixError{"M_ROOM_IN_USE", msg}
}

// ASExclusive is an error returned when an application service tries to
// register an username that is outside of its registered namespace, or if a
// user attempts to register a username or room alias within an exclusive
// namespace.
func ASExclusive(msg string) *MatrixError {
	return &MatrixError{"M_EXCLUSIVE", msg}
}

// GuestAccessForbidden is an error which is returned when the client is
// forbidden from accessing a resource as a guest.
func GuestAccessForbidden(msg string) *MatrixError {
	return &MatrixError{"M_GUEST_ACCESS_FORBIDDEN", msg}
}

// InvalidSignature is an error which is returned when the client tries
// to upload invalid signatures.
func InvalidSignature(msg string) *MatrixError {
	return &MatrixError{"M_INVALID_SIGNATURE", msg}
}

// InvalidParam is an error that is returned when a parameter was invalid,
// traditionally with cross-signing.
func InvalidParam(msg string) *MatrixError {
	return &MatrixError{"M_INVALID_PARAM", msg}
}

// MissingParam is an error that is returned when a parameter was incorrect,
// traditionally with cross-signing.
func MissingParam(msg string) *MatrixError {
	return &MatrixError{"M_MISSING_PARAM", msg}
}

// UnableToAuthoriseJoin is an error that is returned when a server that we
// are trying to join via doesn't know enough to authorise a restricted join.
func UnableToAuthoriseJoin(msg string) *MatrixError {
	return &MatrixError{"M_UNABLE_TO_AUTHORISE_JOIN", msg}
}

// UnableToGrantJoin is an error that is returned when a server that we
// are trying to join via doesn't have a user with power to invite.
func UnableToGrantJoin(msg string) *MatrixError {
	return &MatrixError{"M_UNABLE_TO_GRANT_JOIN", msg}
}

type IncompatibleRoomVersionError struct {
	RoomVersion string `json:"room_version"`
	Error       string `json:"error"`
	Code        string `json:"errcode"`
}

// IncompatibleRoomVersion is an error which is returned when the client
// requests a room with a version that is unsupported.
func IncompatibleRoomVersion(roomVersion gomatrixserverlib.RoomVersion) *IncompatibleRoomVersionError {
	return &IncompatibleRoomVersionError{
		Code:        "M_INCOMPATIBLE_ROOM_VERSION",
		RoomVersion: string(roomVersion),
		Error:       fmt.Sprintf("Your homeserver does not support the features required to join this version %q room", roomVersion),
	}
}

// UnsupportedRoomVersion is an error which is returned when the client
// requests a room with a version that is unsupported.
func UnsupportedRoomVersion(msg string) *MatrixError {
	return &MatrixError{"M_UNSUPPORTED_ROOM_VERSION", msg}
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

// NotTrusted is an error which is returned when the client asks the server to
// proxy a request (e.g. 3PID association) to a server that isn't trusted
func NotTrusted(serverName string) *MatrixError {
	return &MatrixError{
		ErrCode: "M_SERVER_NOT_TRUSTED",
		Err:     fmt.Sprintf("Untrusted server '%s'", serverName),
	}
}
