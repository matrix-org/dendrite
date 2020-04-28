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

package input

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/matrix-org/dendrite/common"
	"github.com/matrix-org/dendrite/federationsender/api"
	"github.com/matrix-org/util"

	rsAPI "github.com/matrix-org/dendrite/roomserver/api"
)

// FederationSenderInputAPI implements api.FederationSenderInputAPI
type FederationSenderInputAPI struct {
	RoomserverInputAPI rsAPI.RoomserverInputAPI
}

// SetupHTTP adds the FederationSenderInputAPI handlers to the http.ServeMux.
func (r *FederationSenderInputAPI) SetupHTTP(servMux *http.ServeMux) {
	servMux.Handle(api.RoomserverInputJoinRequestPath,
		common.MakeInternalAPI("inputJoinRequest", func(req *http.Request) util.JSONResponse {
			var request api.InputJoinRequest
			var response api.InputJoinResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.InputJoinRequest(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
	servMux.Handle(api.RoomserverInputLeaveRequestPath,
		common.MakeInternalAPI("inputLeaveRequest", func(req *http.Request) util.JSONResponse {
			var request api.InputLeaveRequest
			var response api.InputLeaveResponse
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				return util.MessageResponse(http.StatusBadRequest, err.Error())
			}
			if err := r.InputLeaveRequest(req.Context(), &request, &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)
}

// InputJoinRequest implements api.FederationSenderInputAPI
func (r *FederationSenderInputAPI) InputJoinRequest(
	ctx context.Context,
	request *api.InputJoinRequest,
	response *api.InputJoinResponse,
) (err error) {
	return nil
}

// InputLeaveRequest implements api.FederationSenderInputAPI
func (r *FederationSenderInputAPI) InputLeaveRequest(
	ctx context.Context,
	request *api.InputLeaveRequest,
	response *api.InputLeaveResponse,
) (err error) {
	return nil
}
