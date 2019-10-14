// Copyright 2018 Vector Creations Ltd
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

package types

// QueryRequest structure
type QueryRequest struct {
	Timeout    int64                  `json:"timeout"`
	DeviceKeys map[string]interface{} `json:"device_keys"`
	Token      string                 `json:"token"`
}

// QueryResponse structure
type QueryResponse struct {
	Failure    map[string]interface{}                `json:"failures"`
	DeviceKeys map[string]map[string]DeviceKeysQuery `json:"device_keys"`
}

// DeviceKeysQuery structure
type DeviceKeysQuery struct {
	UserID    string                       `json:"user_id"`
	DeviceID  string                       `json:"device_id"`
	Algorithm []string                     `json:"algorithms"`
	Keys      map[string]string            `json:"keys"`
	Signature map[string]map[string]string `json:"signatures"`
	Unsigned  UnsignedDeviceInfo           `json:"unsigned"`
}

// UnsignedDeviceInfo structure
type UnsignedDeviceInfo struct {
	Info string `json:"device_display_name"`
}
