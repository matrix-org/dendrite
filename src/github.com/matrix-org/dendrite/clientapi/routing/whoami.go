// Copyright 2018 New Vector Ltd
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

	"github.com/matrix-org/dendrite/clientapi/auth"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/accounts"
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/common/config"
	"github.com/matrix-org/util"
)

// whoamiResponse represents an response for a `whoami` request
type whoamiResponse struct {
	UserID string `json:"user_id"`
}

// Whoami implements `/account/whoami` which enables client to query owner of the HTTP request.
// https://matrix.org/docs/spec/client_server/r0.3.0.html#get-matrix-client-r0-account-whoami
func Whoami(
	req *http.Request, accountDB *accounts.Database, deviceDB *devices.Database,
	cfg *config.Dendrite,
) util.JSONResponse {

	if req.Method == http.MethodGet {
		user, err := auth.VerifyUserFromRequest(req, accountDB, deviceDB, cfg.Derived.ApplicationServices)

		if err != nil {
			return *err
		}
		util.GetLogger(req.Context()).WithField("user", user).Info("Processing whoami request")

		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: whoamiResponse{
				UserID: user,
			},
		}
	}

	return util.JSONResponse{
		Code: http.StatusMethodNotAllowed,
		JSON: jsonerror.NotFound("Bad method"),
	}
}
