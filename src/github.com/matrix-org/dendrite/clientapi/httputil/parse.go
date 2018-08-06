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

	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// ParseIntParam takes a request and parses the query param to an int
// returns 0 when not present and -1 when parsing failed as int
func ParseIntParam(req *http.Request, param string) (int64, *util.JSONResponse) {
	paramStr := req.URL.Query().Get(param)
	if paramStr == "" {
		return 0, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.MissingArgument(fmt.Sprintf("Param %s not found", param)),
		}
	}
	paramInt, err := strconv.ParseInt(paramStr, 0, 64)
	if err != nil {
		return -1, &util.JSONResponse{
			Code: http.StatusBadRequest,
			JSON: jsonerror.InvalidArgumentValue(
				fmt.Sprintf("Param %s is no valid int (%s)", param, err.Error()),
			),
		}
	}
	return paramInt, nil
}

// ParseTSParam takes a req (typically from an application service) and parses a Time object
// from the req if it exists in the query parameters. If it doesn't exist, the
// current time is returned.
func ParseTSParam(req *http.Request) (time.Time, *util.JSONResponse) {
	ts, err := ParseIntParam(req, "ts")
	if err != nil {
		if ts == 0 {
			// param was not available
			return time.Now(), nil
		}
		return time.Time{}, err
	}

	return time.Unix(ts/1000, 0), nil
}
