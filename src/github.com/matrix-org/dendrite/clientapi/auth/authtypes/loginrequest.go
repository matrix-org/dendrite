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

package authtypes

// LoginRequest represents the request sent by the client
// https://matrix.org/docs/spec/client_server/r0.3.0.html#post-matrix-client-r0-login
type LoginRequest struct {
	Type               LoginType `json:"type"`
	User               string    `json:"user"`
	Medium             string    `json:"medium"`
	Address            string    `json:"address"`
	Password           string    `json:"password"`
	Token              string    `json:"token"`
	DeviceID           string    `json:"device_id"`
	InitialDisplayName *string   `json:"initial_device_display_name"`
}
