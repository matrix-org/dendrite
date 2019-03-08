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

const pathPrefixR0 = "/_matrix/client/r0"

func TagRoom(
	req *http.Request
) util.JSONResponse {

	r0mux := apiMux.PathPrefix(pathPrefixR0).Subrouter()


	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags",
		common.MakeAuthAPI("account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return GetTag()
		}),
	).Methods(http.MethodGet, http.MethodOptions)

	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		common.MakeAuthAPI("account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return PutTag()
		}),
	).Methods(http.MethodPut, http.MethodOptions)

	r0mux.Handle("/user/{userId}/rooms/{roomId}/tags/{tag}",
		common.MakeAuthAPI("account_3pid", authData, func(req *http.Request, device *authtypes.Device) util.JSONResponse {
			return DeleteTag()
		}),
	).Methods(http.MethodDelete, http.MethodOptions)
	
}