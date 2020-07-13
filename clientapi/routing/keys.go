// Copyright 2020 The Matrix.org Foundation C.I.C.
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

package routing

import (
	"net/http"

	"github.com/matrix-org/util"
)

func QueryKeys(
	req *http.Request,
) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: map[string]interface{}{
			"failures":    map[string]interface{}{},
			"device_keys": map[string]interface{}{},
		},
	}
}

func UploadKeys(req *http.Request) util.JSONResponse {
	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
