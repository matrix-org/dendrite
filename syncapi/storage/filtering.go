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

package storage

import (
	"strings"
)

// filterConvertWildcardToSQL converts wildcards as defined in
// https://matrix.org/docs/spec/client_server/r0.3.0.html#post-matrix-client-r0-user-userid-filter
// to SQL wildcards that can be used with LIKE()
func filterConvertTypeWildcardToSQL(values []string) []string {
	ret := make([]string, len(values))
	for i := range values {
		ret[i] = strings.Replace(values[i], "*", "%", -1)
	}
	return ret
}
