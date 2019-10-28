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

// ClaimRequest structure
type ClaimRequest struct {
	OneTimeKeys map[string]map[string]string `json:"one_time_keys"`
}

// ClaimResponse structure
type ClaimResponse struct {
	OneTimeKeys map[string]map[string]map[string]interface{}
}

// KeyObject structure
type KeyObject struct {
	Key        string            `json:"key"`
	Signatures map[string]string `json:"signatures"`
}
