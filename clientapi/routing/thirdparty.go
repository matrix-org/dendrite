// Copyright 2024 New Vector Ltd.
// Copyright 2022 The Matrix.org Foundation C.I.C.
//
// SPDX-License-Identifier: AGPL-3.0-only OR LicenseRef-Element-Commercial
// Please see LICENSE files in the repository root for full details.

package routing

import (
	"net/http"
	"net/url"

	"github.com/matrix-org/util"

	appserviceAPI "github.com/element-hq/dendrite/appservice/api"
	"github.com/element-hq/dendrite/userapi/api"
	"github.com/matrix-org/gomatrixserverlib/spec"
)

// Protocols implements
//
//	GET /_matrix/client/v3/thirdparty/protocols/{protocol}
//	GET /_matrix/client/v3/thirdparty/protocols
func Protocols(req *http.Request, asAPI appserviceAPI.AppServiceInternalAPI, device *api.Device, protocol string) util.JSONResponse {
	resp := &appserviceAPI.ProtocolResponse{}

	if err := asAPI.Protocols(req.Context(), &appserviceAPI.ProtocolRequest{Protocol: protocol}, resp); err != nil {
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !resp.Exists {
		if protocol != "" {
			return util.JSONResponse{
				Code: http.StatusNotFound,
				JSON: spec.NotFound("The protocol is unknown."),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !resp.Exists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("The Matrix User ID was not found"),
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
		return util.JSONResponse{
			Code: http.StatusInternalServerError,
			JSON: spec.InternalServerError{},
		}
	}
	if !resp.Exists {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: spec.NotFound("No portal rooms were found."),
		}
	}
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: resp.Locations,
	}
}
