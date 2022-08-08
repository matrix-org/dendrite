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
	"net/http"

	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/httputil"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/util"
)

// nolint: gocyclo
func AddRoutes(internalAPIMux *mux.Router, s api.UserInternalAPI) {
	addRoutesLoginToken(internalAPIMux, s)

	internalAPIMux.Handle(
		PerformAccountCreationPath,
		httputil.MakeInternalRPCAPI(PerformAccountCreationPath, s.PerformAccountCreation),
	)

	internalAPIMux.Handle(
		PerformPasswordUpdatePath,
		httputil.MakeInternalRPCAPI(PerformPasswordUpdatePath, s.PerformPasswordUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceCreationPath,
		httputil.MakeInternalRPCAPI(PerformDeviceCreationPath, s.PerformDeviceCreation),
	)

	internalAPIMux.Handle(
		PerformLastSeenUpdatePath,
		httputil.MakeInternalRPCAPI(PerformLastSeenUpdatePath, s.PerformLastSeenUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceUpdatePath,
		httputil.MakeInternalRPCAPI(PerformDeviceUpdatePath, s.PerformDeviceUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceDeletionPath,
		httputil.MakeInternalRPCAPI(PerformDeviceDeletionPath, s.PerformDeviceDeletion),
	)

	internalAPIMux.Handle(
		PerformAccountDeactivationPath,
		httputil.MakeInternalRPCAPI(PerformAccountDeactivationPath, s.PerformAccountDeactivation),
	)

	internalAPIMux.Handle(
		PerformOpenIDTokenCreationPath,
		httputil.MakeInternalRPCAPI(PerformOpenIDTokenCreationPath, s.PerformOpenIDTokenCreation),
	)

	internalAPIMux.Handle(
		QueryProfilePath,
		httputil.MakeInternalRPCAPI(QueryProfilePath, s.QueryProfile),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI(QueryAccessTokenPath, s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI(QueryAccessTokenPath, s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryDevicesPath,
		httputil.MakeInternalRPCAPI(QueryDevicesPath, s.QueryDevices),
	)

	internalAPIMux.Handle(
		QueryAccountDataPath,
		httputil.MakeInternalRPCAPI(QueryAccountDataPath, s.QueryAccountData),
	)

	internalAPIMux.Handle(
		QueryDeviceInfosPath,
		httputil.MakeInternalRPCAPI(QueryDeviceInfosPath, s.QueryDeviceInfos),
	)

	internalAPIMux.Handle(
		QuerySearchProfilesPath,
		httputil.MakeInternalRPCAPI(QuerySearchProfilesPath, s.QuerySearchProfiles),
	)

	internalAPIMux.Handle(
		QueryOpenIDTokenPath,
		httputil.MakeInternalRPCAPI(QueryOpenIDTokenPath, s.QueryOpenIDToken),
	)

	internalAPIMux.Handle(
		InputAccountDataPath,
		httputil.MakeInternalRPCAPI(InputAccountDataPath, s.InputAccountData),
	)

	internalAPIMux.Handle(
		QueryKeyBackupPath,
		httputil.MakeInternalRPCAPI(QueryKeyBackupPath, s.QueryKeyBackup),
	)

	internalAPIMux.Handle(
		PerformKeyBackupPath,
		httputil.MakeInternalRPCAPI(PerformKeyBackupPath, s.PerformKeyBackup),
	)

	internalAPIMux.Handle(
		QueryNotificationsPath,
		httputil.MakeInternalRPCAPI(QueryNotificationsPath, s.QueryNotifications),
	)

	internalAPIMux.Handle(
		PerformPusherSetPath,
		httputil.MakeInternalRPCAPI(PerformPusherSetPath, s.PerformPusherSet),
	)

	internalAPIMux.Handle(
		PerformPusherDeletionPath,
		httputil.MakeInternalRPCAPI(PerformPusherDeletionPath, s.PerformPusherDeletion),
	)

	internalAPIMux.Handle(
		QueryPushersPath,
		httputil.MakeInternalRPCAPI(QueryPushersPath, s.QueryPushers),
	)

	internalAPIMux.Handle(
		PerformPushRulesPutPath,
		httputil.MakeInternalRPCAPI(PerformPushRulesPutPath, s.PerformPushRulesPut),
	)

	internalAPIMux.Handle(
		QueryPushRulesPath,
		httputil.MakeInternalRPCAPI(QueryPushRulesPath, s.QueryPushRules),
	)

	internalAPIMux.Handle(
		PerformSetAvatarURLPath,
		httputil.MakeInternalRPCAPI(PerformSetAvatarURLPath, s.SetAvatarURL),
	)

	// TODO: Look at the shape of this
	internalAPIMux.Handle(QueryNumericLocalpartPath,
		httputil.MakeInternalAPI(QueryNumericLocalpartPath, func(req *http.Request) util.JSONResponse {
			response := api.QueryNumericLocalpartResponse{}
			if err := s.QueryNumericLocalpart(req.Context(), &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(
		QueryAccountAvailabilityPath,
		httputil.MakeInternalRPCAPI(QueryAccountAvailabilityPath, s.QueryAccountAvailability),
	)

	internalAPIMux.Handle(
		QueryAccountByPasswordPath,
		httputil.MakeInternalRPCAPI(QueryAccountByPasswordPath, s.QueryAccountByPassword),
	)

	internalAPIMux.Handle(
		PerformSetDisplayNamePath,
		httputil.MakeInternalRPCAPI(PerformSetDisplayNamePath, s.SetDisplayName),
	)

	internalAPIMux.Handle(
		QueryLocalpartForThreePIDPath,
		httputil.MakeInternalRPCAPI(QueryLocalpartForThreePIDPath, s.QueryLocalpartForThreePID),
	)

	internalAPIMux.Handle(
		QueryThreePIDsForLocalpartPath,
		httputil.MakeInternalRPCAPI(QueryThreePIDsForLocalpartPath, s.QueryThreePIDsForLocalpart),
	)

	internalAPIMux.Handle(
		PerformForgetThreePIDPath,
		httputil.MakeInternalRPCAPI(PerformForgetThreePIDPath, s.PerformForgetThreePID),
	)

	internalAPIMux.Handle(
		PerformSaveThreePIDAssociationPath,
		httputil.MakeInternalRPCAPI(PerformSaveThreePIDAssociationPath, s.PerformSaveThreePIDAssociation),
	)
}
