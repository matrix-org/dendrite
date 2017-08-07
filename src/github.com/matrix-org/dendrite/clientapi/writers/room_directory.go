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

package writers

import (
	"net/http"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
)

type roomDirectoryVisibility struct {
	Visibility string `json:"visibility"`
}

// GetVisibility implements GET /directory/list/room/{roomID}
func GetVisibility(
	req *http.Request, roomID string,
) util.JSONResponse {
	return util.JSONResponse{
		Code: 200,
		JSON: roomDirectoryVisibility{"public"},
	}
}

// SetVisibility implements PUT /directory/list/room/{roomID}
func SetVisibility(
	req *http.Request, device authtypes.Device, roomID string,
	cfg config.Dendrite,
) util.JSONResponse {
	return util.JSONResponse{
		Code: 200,
		JSON: struct{}{},
	}
}
