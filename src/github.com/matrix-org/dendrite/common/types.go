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

package common

import (
	"strconv"
)

// AccountData represents account data sent from the client API server to the
// sync API server
type AccountData struct {
	RoomID string `json:"room_id"`
	Type   string `json:"type"`
}

// ProfileResponse is a struct containing all known user profile data
type ProfileResponse struct {
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"displayname"`
}

// AvatarURL is a struct containing only the URL to a user's avatar
type AvatarURL struct {
	AvatarURL string `json:"avatar_url"`
}

// DisplayName is a struct containing only a user's display name
type DisplayName struct {
	DisplayName string `json:"displayname"`
}

// WeakBoolean is a type that will Unmarshal to true or false even if the encoded
// representation is "true"/1 or "false"/0, as well as whatever other forms are
// recognized by strconv.ParseBool
type WeakBoolean bool

// MTag contains the data for a Tag which can be referenced by the Tag name
type MTag struct {
	Tags map[string]TagProperties `json:"tags"`
}

// TagProperties contains the properties of an MTag datatype
type TagProperties struct {
	Order float32 `json:"order,omitempty"` // Empty values must be neglected
}

// UnmarshalJSON is overridden here to allow strings vaguely representing a true
// or false boolean to be set as their closest counterpart
func (b *WeakBoolean) UnmarshalJSON(data []byte) error {
	result, err := strconv.ParseBool(string(data))
	if err != nil {
		return err
	}

	// Set boolean value based on string input
	*b = WeakBoolean(result)

	return nil
}
