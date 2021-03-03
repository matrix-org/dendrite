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

package routing

import (
	"net/http"

	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/util"
)

// mediaConfigResponse represents a response for a `config` request
type mediaConfigResponse struct {
	MaxFileSizeBytes *config.FileSizeBytes `json:"m.upload.size"`
}

// Config implements `/config` which allows clients to retrieve the
// configuration of the content repository, such as upload limitations.
// https://matrix.org/docs/spec/client_server/latest#get-matrix-media-r0-config
func Config(req *http.Request, cfg *config.MediaAPI) util.JSONResponse {
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: mediaConfigResponse{MaxFileSizeBytes: cfg.MaxFileSizeBytes},
	}
}
