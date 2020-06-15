// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package api

import "context"

// UserInternalAPI is the internal API for information about users and devices.
type UserInternalAPI interface {
	QueryProfile(ctx context.Context, req *QueryProfileRequest, res *QueryProfileResponse) error
}

// QueryProfileRequest is the request for QueryProfile
type QueryProfileRequest struct {
	// The user ID to query
	UserID string
}

// QueryProfileResponse is the response for QueryProfile
type QueryProfileResponse struct {
	// True if the user has been created. Querying for a profile does not create them.
	UserExists bool
	// The current display name if set.
	DisplayName string
	// The current avatar URL if set.
	AvatarURL string
}
