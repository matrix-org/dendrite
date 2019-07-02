// Copyright 2019 The Matrix.org Foundation C.I.C.
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
	"net/url"
)

// URLDecodeVarMap is a function that iterates through each of the items in a
// map, URL decodes the values, and returns a new map with the decoded values
// under the same key names
func URLDecodeVarMap(vars map[string]string) (map[string]string, error) {
	decodedVars := make(map[string]string, len(vars))
	for key, value := range vars {
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return make(map[string]string), err
		}
		decodedVars[key] = decoded
	}

	return decodedVars, err
}
