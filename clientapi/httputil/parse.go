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

package httputil

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ParseTSParam takes a req (typically from an application service) and parses a Time object
// from the req if it exists in the query parameters. If it doesn't exist, the
// current time is returned.
func ParseTSParam(req *http.Request) (time.Time, error) {
	// Use the ts parameter's value for event time if present
	tsStr := req.URL.Query().Get("ts")
	if tsStr == "" {
		return time.Now(), nil
	}

	// The parameter exists, parse into a Time object
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("param 'ts' is no valid int (%s)", err.Error())
	}

	return time.Unix(ts/1000, 0), nil
}
