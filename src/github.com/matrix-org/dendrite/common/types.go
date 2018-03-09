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

// AccountData represents account data sent from the client API server to the
// sync API server
type AccountData struct {
	RoomID string `json:"room_id"`
	Type   string `json:"type"`
	Sender string `json:"sender"`
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
