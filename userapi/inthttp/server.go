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
		httputil.MakeInternalRPCAPI("PerformAccountCreation", s.PerformAccountCreation),
	)

	internalAPIMux.Handle(
		PerformPasswordUpdatePath,
		httputil.MakeInternalRPCAPI("PerformPasswordUpdate", s.PerformPasswordUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceCreationPath,
		httputil.MakeInternalRPCAPI("PerformDeviceCreation", s.PerformDeviceCreation),
	)

	internalAPIMux.Handle(
		PerformLastSeenUpdatePath,
		httputil.MakeInternalRPCAPI("PerformLastSeenUpdate", s.PerformLastSeenUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceUpdatePath,
		httputil.MakeInternalRPCAPI("PerformDeviceUpdate", s.PerformDeviceUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceDeletionPath,
		httputil.MakeInternalRPCAPI("PerformDeviceDeletion", s.PerformDeviceDeletion),
	)

	internalAPIMux.Handle(
		PerformAccountDeactivationPath,
		httputil.MakeInternalRPCAPI("PerformAccountDeactivation", s.PerformAccountDeactivation),
	)

	internalAPIMux.Handle(
		PerformOpenIDTokenCreationPath,
		httputil.MakeInternalRPCAPI("PerformOpenIDTokenCreation", s.PerformOpenIDTokenCreation),
	)

	internalAPIMux.Handle(
		QueryProfilePath,
		httputil.MakeInternalRPCAPI("QueryProfile", s.QueryProfile),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI("QueryAccessToken", s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI("QueryAccessToken", s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryDevicesPath,
		httputil.MakeInternalRPCAPI("QueryDevices", s.QueryDevices),
	)

	internalAPIMux.Handle(
		QueryAccountDataPath,
		httputil.MakeInternalRPCAPI("QueryAccountData", s.QueryAccountData),
	)

	internalAPIMux.Handle(
		QueryDeviceInfosPath,
		httputil.MakeInternalRPCAPI("QueryDeviceInfos", s.QueryDeviceInfos),
	)

	internalAPIMux.Handle(
		QuerySearchProfilesPath,
		httputil.MakeInternalRPCAPI("QuerySearchProfiles", s.QuerySearchProfiles),
	)

	internalAPIMux.Handle(
		QueryOpenIDTokenPath,
		httputil.MakeInternalRPCAPI("QueryOpenIDToken", s.QueryOpenIDToken),
	)

	internalAPIMux.Handle(
		InputAccountDataPath,
		httputil.MakeInternalRPCAPI("InputAccountData", s.InputAccountData),
	)

	internalAPIMux.Handle(
		QueryKeyBackupPath,
		httputil.MakeInternalRPCAPI("QueryKeyBackup", s.QueryKeyBackup),
	)

	internalAPIMux.Handle(
		PerformKeyBackupPath,
		httputil.MakeInternalRPCAPI("PerformKeyBackup", s.PerformKeyBackup),
	)

	internalAPIMux.Handle(
		QueryNotificationsPath,
		httputil.MakeInternalRPCAPI("QueryNotifications", s.QueryNotifications),
	)

	internalAPIMux.Handle(
		PerformPusherSetPath,
		httputil.MakeInternalRPCAPI("PerformPusherSet", s.PerformPusherSet),
	)

	internalAPIMux.Handle(
		PerformPusherDeletionPath,
		httputil.MakeInternalRPCAPI("PerformPusherDeletion", s.PerformPusherDeletion),
	)

	internalAPIMux.Handle(
		QueryPushersPath,
		httputil.MakeInternalRPCAPI("QueryPushers", s.QueryPushers),
	)

	internalAPIMux.Handle(
		PerformPushRulesPutPath,
		httputil.MakeInternalRPCAPI("PerformPushRulesPut", s.PerformPushRulesPut),
	)

	internalAPIMux.Handle(
		QueryPushRulesPath,
		httputil.MakeInternalRPCAPI("QueryPushRules", s.QueryPushRules),
	)

	internalAPIMux.Handle(
		PerformSetAvatarURLPath,
		httputil.MakeInternalRPCAPI("PerformSetAvatarURL", s.SetAvatarURL),
	)

	// TODO: Look at the shape of this
	internalAPIMux.Handle(QueryNumericLocalpartPath,
		httputil.MakeInternalAPI("queryNumericLocalpart", func(req *http.Request) util.JSONResponse {
			response := api.QueryNumericLocalpartResponse{}
			if err := s.QueryNumericLocalpart(req.Context(), &response); err != nil {
				return util.ErrorResponse(err)
			}
			return util.JSONResponse{Code: http.StatusOK, JSON: &response}
		}),
	)

	internalAPIMux.Handle(
		QueryAccountAvailabilityPath,
		httputil.MakeInternalRPCAPI("QueryAccountAvailability", s.QueryAccountAvailability),
	)

	internalAPIMux.Handle(
		QueryAccountByPasswordPath,
		httputil.MakeInternalRPCAPI("QueryAccountByPassword", s.QueryAccountByPassword),
	)

	internalAPIMux.Handle(
		PerformSetDisplayNamePath,
		httputil.MakeInternalRPCAPI("SetDisplayName", s.SetDisplayName),
	)

	internalAPIMux.Handle(
		QueryLocalpartForThreePIDPath,
		httputil.MakeInternalRPCAPI("QueryLocalpartForThreePID", s.QueryLocalpartForThreePID),
	)

	internalAPIMux.Handle(
		QueryThreePIDsForLocalpartPath,
		httputil.MakeInternalRPCAPI("QueryThreePIDsForLocalpart", s.QueryThreePIDsForLocalpart),
	)

	internalAPIMux.Handle(
		PerformForgetThreePIDPath,
		httputil.MakeInternalRPCAPI("PerformForgetThreePID", s.PerformForgetThreePID),
	)

	internalAPIMux.Handle(
		PerformSaveThreePIDAssociationPath,
		httputil.MakeInternalRPCAPI("PerformSaveThreePIDAssociation", s.PerformSaveThreePIDAssociation),
	)
}
