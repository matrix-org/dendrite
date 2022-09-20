// Copyright 2020 The Matrix.org Foundation C.I.C.
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
	"github.com/matrix-org/dendrite/keyserver/api"
)

func AddRoutes(internalAPIMux *mux.Router, s api.KeyInternalAPI) {
	internalAPIMux.Handle(
		PerformClaimKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverPerformClaimKeys", s.PerformClaimKeys),
	)

	internalAPIMux.Handle(
		PerformDeleteKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverPerformDeleteKeys", s.PerformDeleteKeys),
	)

	internalAPIMux.Handle(
		PerformUploadKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverPerformUploadKeys", s.PerformUploadKeys),
	)

	internalAPIMux.Handle(
		PerformUploadDeviceKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverPerformUploadDeviceKeys", s.PerformUploadDeviceKeys),
	)

	internalAPIMux.Handle(
		PerformUploadDeviceSignaturesPath,
		httputil.MakeInternalRPCAPI("KeyserverPerformUploadDeviceSignatures", s.PerformUploadDeviceSignatures),
	)

	internalAPIMux.Handle(
		QueryKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverQueryKeys", s.QueryKeys),
	)

	internalAPIMux.Handle(
		QueryOneTimeKeysPath,
		httputil.MakeInternalRPCAPI("KeyserverQueryOneTimeKeys", s.QueryOneTimeKeys),
	)

	internalAPIMux.Handle(
		QueryDeviceMessagesPath,
		httputil.MakeInternalRPCAPI("KeyserverQueryDeviceMessages", s.QueryDeviceMessages),
	)

	internalAPIMux.Handle(
		QueryKeyChangesPath,
		httputil.MakeInternalRPCAPI("KeyserverQueryKeyChanges", s.QueryKeyChanges),
	)

	internalAPIMux.Handle(
		QuerySignaturesPath,
		httputil.MakeInternalRPCAPI("KeyserverQuerySignatures", s.QuerySignatures),
	)

	internalAPIMux.Handle(
		PerformMarkAsStalePath,
		httputil.MakeInternalRPCAPI("KeyserverMarkAsStale", s.PerformMarkAsStaleIfNeeded),
	)
}
