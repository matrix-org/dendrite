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

package readers

import (
	"fmt"
	"github.com/matrix-org/dendrite/clientapi/config"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
	"net/http"
)

type loginFlows struct {
	Flows []flow `json:"flows"`
}

type flow struct {
	Type   string   `json:"type"`
	Stages []string `json:"stages"`
}

type passwordRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type loginResponse struct {
	UserID      string `json:"user_id"`
	AccessToken string `json:"access_token"`
	HomeServer  string `json:"home_server"`
}

func passwordLogin() loginFlows {
	f := loginFlows{}
	s := flow{"m.login.password", []string{"m.login.password"}}
	f.Flows = append(f.Flows, s)
	return f
}

// Login implements GET and POST /login
func Login(req *http.Request, cfg config.ClientAPI) util.JSONResponse {
	if req.Method == "GET" { // TODO: support other forms of login other than password, depending on config options
		return util.JSONResponse{
			Code: 200,
			JSON: passwordLogin(),
		}
	} else if req.Method == "POST" {
		var r passwordRequest
		resErr := httputil.UnmarshalJSONRequest(req, &r)
		if resErr != nil {
			return *resErr
		}
		if r.User == "" {
			return util.JSONResponse{
				Code: 400,
				JSON: jsonerror.BadJSON("'user' must be supplied."),
			}
		}
		// TODO: Check username and password properly
		return util.JSONResponse{
			Code: 200,
			JSON: loginResponse{
				UserID:      makeUserID(r.User, cfg.ServerName),
				AccessToken: makeUserID(r.User, cfg.ServerName), // FIXME: token is the user ID for now
				HomeServer:  cfg.ServerName,
			},
		}
	}
	return util.JSONResponse{
		Code: 405,
		JSON: jsonerror.NotFound("Bad method"),
	}
}

func makeUserID(localpart, domain string) string {
	return fmt.Sprintf("@%s:%s", localpart, domain)
}
