// Copyright 2019 Sumukha PK
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

// QueryRequest is the request for /query
type QueryRequest struct {
	DeviceKeys map[string][]string `json:"device_keys"`
}

// UnsignedDeviceInfo is the struct for UDI
type UnsignedDeviceInfo struct {
	DeviceDisplayName string `json:"device_display_name"`
}

// DeviceKeys has the data of the keys of the device
type DeviceKeys struct {
	UserID     string                       `json:"user_id"`
	DeviceID   string                       `json:"edvice_id"`
	Algorithms []string                     `json:"algorithms"`
	Keys       map[string]string            `json:"keys"`
	Signatures map[string]map[string]string `json:"signatures"`
	Unsigned   UnsignedDeviceInfo           `json:"unsigned"`
}

// QueryResponse is the response for /query
type QueryResponse struct {
	DeviceKeys map[string]map[string]DeviceKeys `json:"device_keys"`
}

// KeyHolder structure
type KeyHolder struct {
	UserID,
	DeviceID,
	Signature,
	KeyAlgorithm,
	KeyID,
	Key,
	KeyType string
}
