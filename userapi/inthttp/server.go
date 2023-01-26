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
	"github.com/matrix-org/dendrite/userapi/api"
)

// nolint: gocyclo
func AddRoutes(internalAPIMux *mux.Router, s api.UserInternalAPI, enableMetrics bool) {
	addRoutesLoginToken(internalAPIMux, s, enableMetrics)

	internalAPIMux.Handle(
		PerformAccountCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformAccountCreation", enableMetrics, s.PerformAccountCreation),
	)

	internalAPIMux.Handle(
		PerformPasswordUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPasswordUpdate", enableMetrics, s.PerformPasswordUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceCreation", enableMetrics, s.PerformDeviceCreation),
	)

	internalAPIMux.Handle(
		PerformLastSeenUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformLastSeenUpdate", enableMetrics, s.PerformLastSeenUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceUpdate", enableMetrics, s.PerformDeviceUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceDeletionPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceDeletion", enableMetrics, s.PerformDeviceDeletion),
	)

	internalAPIMux.Handle(
		PerformAccountDeactivationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformAccountDeactivation", enableMetrics, s.PerformAccountDeactivation),
	)

	internalAPIMux.Handle(
		PerformOpenIDTokenCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformOpenIDTokenCreation", enableMetrics, s.PerformOpenIDTokenCreation),
	)

	internalAPIMux.Handle(
		QueryProfilePath,
		httputil.MakeInternalRPCAPI("UserAPIQueryProfile", enableMetrics, s.QueryProfile),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccessToken", enableMetrics, s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryDevicesPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryDevices", enableMetrics, s.QueryDevices),
	)

	internalAPIMux.Handle(
		QueryAccountDataPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountData", enableMetrics, s.QueryAccountData),
	)

	internalAPIMux.Handle(
		QueryDeviceInfosPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryDeviceInfos", enableMetrics, s.QueryDeviceInfos),
	)

	internalAPIMux.Handle(
		QuerySearchProfilesPath,
		httputil.MakeInternalRPCAPI("UserAPIQuerySearchProfiles", enableMetrics, s.QuerySearchProfiles),
	)

	internalAPIMux.Handle(
		QueryOpenIDTokenPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryOpenIDToken", enableMetrics, s.QueryOpenIDToken),
	)

	internalAPIMux.Handle(
		InputAccountDataPath,
		httputil.MakeInternalRPCAPI("UserAPIInputAccountData", enableMetrics, s.InputAccountData),
	)

	internalAPIMux.Handle(
		QueryKeyBackupPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryKeyBackup", enableMetrics, s.QueryKeyBackup),
	)

	internalAPIMux.Handle(
		PerformKeyBackupPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformKeyBackup", enableMetrics, s.PerformKeyBackup),
	)

	internalAPIMux.Handle(
		QueryNotificationsPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryNotifications", enableMetrics, s.QueryNotifications),
	)

	internalAPIMux.Handle(
		PerformPusherSetPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPusherSet", enableMetrics, s.PerformPusherSet),
	)

	internalAPIMux.Handle(
		PerformPusherDeletionPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPusherDeletion", enableMetrics, s.PerformPusherDeletion),
	)

	internalAPIMux.Handle(
		QueryPushersPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryPushers", enableMetrics, s.QueryPushers),
	)

	internalAPIMux.Handle(
		PerformPushRulesPutPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPushRulesPut", enableMetrics, s.PerformPushRulesPut),
	)

	internalAPIMux.Handle(
		QueryPushRulesPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryPushRules", enableMetrics, s.QueryPushRules),
	)

	internalAPIMux.Handle(
		PerformSetAvatarURLPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformSetAvatarURL", enableMetrics, s.SetAvatarURL),
	)

	internalAPIMux.Handle(
		QueryNumericLocalpartPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryNumericLocalpart", enableMetrics, s.QueryNumericLocalpart),
	)

	internalAPIMux.Handle(
		QueryAccountAvailabilityPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountAvailability", enableMetrics, s.QueryAccountAvailability),
	)

	internalAPIMux.Handle(
		QueryAccountByPasswordPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountByPassword", enableMetrics, s.QueryAccountByPassword),
	)

	internalAPIMux.Handle(
		PerformSetDisplayNamePath,
		httputil.MakeInternalRPCAPI("UserAPISetDisplayName", enableMetrics, s.SetDisplayName),
	)

	internalAPIMux.Handle(
		QueryLocalpartForThreePIDPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryLocalpartForThreePID", enableMetrics, s.QueryLocalpartForThreePID),
	)

	internalAPIMux.Handle(
		QueryThreePIDsForLocalpartPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryThreePIDsForLocalpart", enableMetrics, s.QueryThreePIDsForLocalpart),
	)

	internalAPIMux.Handle(
		PerformForgetThreePIDPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformForgetThreePID", enableMetrics, s.PerformForgetThreePID),
	)

	internalAPIMux.Handle(
		PerformSaveThreePIDAssociationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformSaveThreePIDAssociation", enableMetrics, s.PerformSaveThreePIDAssociation),
	)

	internalAPIMux.Handle(
		QueryAccountByLocalpartPath,
		httputil.MakeInternalRPCAPI("AccountByLocalpart", enableMetrics, s.QueryAccountByLocalpart),
	)
}
