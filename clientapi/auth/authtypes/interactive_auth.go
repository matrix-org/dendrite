// Copyright Piotr Kozimor <piotr.kozimor@globekeeper.com>
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

package authtypes

type InteractiveAuth struct {
	// Flows is a slice of flows, which represent one possible way that the client can authenticate a request.
	// http://matrix.org/docs/spec/HEAD/client_server/r0.3.0.html#user-interactive-authentication-api
	// As long as the generated flows only rely on config file options,
	// we can generate them on startup and store them until needed
	Flows []Flow `json:"flows"`

	// Params that need to be returned to the client during
	// registration in order to complete registration stages.
	Params map[string]interface{} `json:"params"`
}
