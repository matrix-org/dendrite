// Copyright 2021 The Matrix.org Foundation C.I.C.
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

package inthttp

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
)

// addRoutesLoginToken adds routes for all login token API calls.
func addRoutesLoginToken(internalAPIMux *mux.Router, s api.UserInternalAPI) {
	internalAPIMux.Handle(
		PerformLoginTokenCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformLoginTokenCreation", s.PerformLoginTokenCreation),
	)

	internalAPIMux.Handle(
		PerformLoginTokenDeletionPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformLoginTokenDeletion", s.PerformLoginTokenDeletion),
	)

	internalAPIMux.Handle(
		QueryLoginTokenPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryLoginToken", s.QueryLoginToken),
	)
}
