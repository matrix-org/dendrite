// Copyright 2022 The Matrix.org Foundation C.I.C.
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
	"net/url"

	"github.com/matrix-org/util"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/dendrite/userapi/api"
)

// Protocols implements
//
//	GET /_matrix/client/v3/thirdparty/protocols/{protocol}
//	GET /_matrix/client/v3/thirdparty/protocols
func Protocols(req *http.Request, asAPI appserviceAPI.AppServiceInternalAPI, device *api.Device, protocol string) util.JSONResponse {
	resp := &appserviceAPI.ProtocolResponse{}

	if err := asAPI.Protocols(req.Context(), &appserviceAPI.ProtocolRequest{Protocol: protocol}, resp); err != nil {
		return jsonerror.InternalServerError()
	}
	if !resp.Exists {
		if protocol != "" {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: jsonerror.NotFound("The protocol is unknown."),
			}
		}
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: struct{}{},
		}
	}
	if protocol != "" {
		return util.JSONResponse{
			Code: http.StatusOK,
			JSON: resp.Protocols[protocol],
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp.Protocols,
	}
}

// User implements
//
//	GET /_matrix/client/v3/thirdparty/user
//	GET /_matrix/client/v3/thirdparty/user/{protocol}
func User(req *http.Request, asAPI appserviceAPI.AppServiceInternalAPI, device *api.Device, protocol string, params url.Values) util.JSONResponse {
	resp := &appserviceAPI.UserResponse{}

	params.Del("access_token")
	if err := asAPI.User(req.Context(), &appserviceAPI.UserRequest{
		Protocol: protocol,
		Params:   params.Encode(),
	}, resp); err != nil {
		return jsonerror.InternalServerError()
	}
	if !resp.Exists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("The Matrix User ID was not found"),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp.Users,
	}
}

// Location implements
//
//	GET /_matrix/client/v3/thirdparty/location
//	GET /_matrix/client/v3/thirdparty/location/{protocol}
func Location(req *http.Request, asAPI appserviceAPI.AppServiceInternalAPI, device *api.Device, protocol string, params url.Values) util.JSONResponse {
	resp := &appserviceAPI.LocationResponse{}

	params.Del("access_token")
	if err := asAPI.Locations(req.Context(), &appserviceAPI.LocationRequest{
		Protocol: protocol,
		Params:   params.Encode(),
	}, resp); err != nil {
		return jsonerror.InternalServerError()
	}
	if !resp.Exists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("No portal rooms were found."),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp.Locations,
	}
}
