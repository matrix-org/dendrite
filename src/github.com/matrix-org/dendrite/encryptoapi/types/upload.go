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

// UploadEncrypt structure
type UploadEncrypt struct {
	DeviceKeys DeviceKeys             `json:"device_keys"`
	OneTimeKey map[string]interface{} `json:"one_time_keys"`
}

// UploadEncryptSpecific structure
type UploadEncryptSpecific struct {
	DeviceKeys DeviceKeys         `json:"device_keys"`
	OneTimeKey OneTimeKeySpecific `json:"one_time_keys"`
}

// UploadResponse structure
type UploadResponse struct {
	Count map[string]int `json:"one_time_key_counts"`
}

// DeviceKeys structure
type DeviceKeys struct {
	UserID    string                       `json:"user_id"`
	DeviceID  string                       `json:"device_id"`
	Algorithm []string                     `json:"algorithms"`
	Keys      map[string]string            `json:"keys"`
	Signature map[string]map[string]string `json:"signatures"`
}

// KeyObject structure
type KeyObject struct {
	Key       string                       `json:"key"`
	Signature map[string]map[string]string `json:"signatures"`
}

// OneTimeKey structure
type OneTimeKey struct {
	//KeyString map[string]string
	//KeyObject map[string]KeyObject
	KeySth map[string]interface{}
}

// OneTimeKeySpecific structure
type OneTimeKeySpecific struct {
	KeyString map[string]string
	KeyObject map[string]KeyObject
	//KeySth map[string]interface{}
}
