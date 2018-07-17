// Copyright 2018 Vector Creations Ltd
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
	"io/ioutil"
	"net/http"
	"strings"

	appserviceAPI "github.com/matrix-org/dendrite/appservice/api"
	appserviceRouting "github.com/matrix-org/dendrite/appservice/routing"
	"github.com/matrix-org/dendrite/clientapi/httputil"
	"github.com/matrix-org/dendrite/clientapi/jsonerror"
	"github.com/matrix-org/util"
)

// GetThirdPartyProtocol returns the protocol definition of a single, given
// protocol ID
func GetThirdPartyProtocol(
	req *http.Request,
	asAPI appserviceAPI.AppServiceQueryAPI,
	protocolID string,
) util.JSONResponse {
	// Retrieve a single protocol definition from the appservice component
	queryReq := appserviceAPI.GetProtocolDefinitionRequest{
		ProtocolID: protocolID,
	}
	var queryRes appserviceAPI.GetProtocolDefinitionResponse
	if err := asAPI.GetProtocolDefinition(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	// Account for unknown protocol/empty definition
	if len(queryRes.ProtocolDefinition) == 0 {
		return util.JSONResponse{
			Code: http.StatusNotFound,
			JSON: jsonerror.NotFound("unknown protocol"),
		}
	}

	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes.ProtocolDefinition,
	}
}

// GetThirdPartyProtocols returns all known third party protocols provided by
// application services connected to this homeserver
func GetThirdPartyProtocols(
	req *http.Request,
	asAPI appserviceAPI.AppServiceQueryAPI,
) util.JSONResponse {
	// Retrieve all known protocols from appservice component
	queryReq := appserviceAPI.GetAllProtocolDefinitionsRequest{}
	var queryRes appserviceAPI.GetAllProtocolDefinitionsResponse
	if err := asAPI.GetAllProtocolDefinitions(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	// TODO: Check what we get if no protocols defined by anyone

	// Return protocol IDs along with definitions
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes.Protocols,
	}
}

// ThirdPartyProxy proxies a third party lookup request to the handler
// application service
func ThirdPartyProxy(
	req *http.Request,
	asAPI appserviceAPI.AppServiceQueryAPI,
	protocolID string,
) util.JSONResponse {
	// Read the request body
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return httputil.LogThenError(req, err)
	}

	// Rewrite the path from a client URL to an application service URL
	requestPath := strings.Replace(req.URL.String(), PathPrefixClient, appserviceRouting.PathPrefixAppUnstable, 1)

	// Proxy the location lookup to the appservices component, which will send it
	// off to an application service
	queryReq := appserviceAPI.ThirdPartyProxyRequest{
		ProtocolID: protocolID,
		Path:       requestPath,
		Content:    body,
	}
	var queryRes appserviceAPI.ThirdPartyProxyResponse
	if err := asAPI.ThirdPartyProxy(req.Context(), &queryReq, &queryRes); err != nil {
		return httputil.LogThenError(req, err)
	}

	// Return response to the client
	return util.JSONResponse{
		Code: http.StatusOK,
		JSON: queryRes.Content,
	}
}
