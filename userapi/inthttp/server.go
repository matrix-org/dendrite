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
func AddRoutes(internalAPIMux *mux.Router, s api.UserInternalAPI) {
	addRoutesLoginToken(internalAPIMux, s)

	internalAPIMux.Handle(
		PerformAccountCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformAccountCreation", s.PerformAccountCreation),
	)

	internalAPIMux.Handle(
		PerformPasswordUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPasswordUpdate", s.PerformPasswordUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceCreation", s.PerformDeviceCreation),
	)

	internalAPIMux.Handle(
		PerformLastSeenUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformLastSeenUpdate", s.PerformLastSeenUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceUpdatePath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceUpdate", s.PerformDeviceUpdate),
	)

	internalAPIMux.Handle(
		PerformDeviceDeletionPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformDeviceDeletion", s.PerformDeviceDeletion),
	)

	internalAPIMux.Handle(
		PerformAccountDeactivationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformAccountDeactivation", s.PerformAccountDeactivation),
	)

	internalAPIMux.Handle(
		PerformOpenIDTokenCreationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformOpenIDTokenCreation", s.PerformOpenIDTokenCreation),
	)

	internalAPIMux.Handle(
		QueryProfilePath,
		httputil.MakeInternalRPCAPI("UserAPIQueryProfile", s.QueryProfile),
	)

	internalAPIMux.Handle(
		QueryAccessTokenPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccessToken", s.QueryAccessToken),
	)

	internalAPIMux.Handle(
		QueryDevicesPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryDevices", s.QueryDevices),
	)

	internalAPIMux.Handle(
		QueryAccountDataPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountData", s.QueryAccountData),
	)

	internalAPIMux.Handle(
		QueryDeviceInfosPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryDeviceInfos", s.QueryDeviceInfos),
	)

	internalAPIMux.Handle(
		QuerySearchProfilesPath,
		httputil.MakeInternalRPCAPI("UserAPIQuerySearchProfiles", s.QuerySearchProfiles),
	)

	internalAPIMux.Handle(
		QueryOpenIDTokenPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryOpenIDToken", s.QueryOpenIDToken),
	)

	internalAPIMux.Handle(
		InputAccountDataPath,
		httputil.MakeInternalRPCAPI("UserAPIInputAccountData", s.InputAccountData),
	)

	internalAPIMux.Handle(
		QueryKeyBackupPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryKeyBackup", s.QueryKeyBackup),
	)

	internalAPIMux.Handle(
		PerformKeyBackupPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformKeyBackup", s.PerformKeyBackup),
	)

	internalAPIMux.Handle(
		QueryNotificationsPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryNotifications", s.QueryNotifications),
	)

	internalAPIMux.Handle(
		PerformPusherSetPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPusherSet", s.PerformPusherSet),
	)

	internalAPIMux.Handle(
		PerformPusherDeletionPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPusherDeletion", s.PerformPusherDeletion),
	)

	internalAPIMux.Handle(
		QueryPushersPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryPushers", s.QueryPushers),
	)

	internalAPIMux.Handle(
		PerformPushRulesPutPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformPushRulesPut", s.PerformPushRulesPut),
	)

	internalAPIMux.Handle(
		QueryPushRulesPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryPushRules", s.QueryPushRules),
	)

	internalAPIMux.Handle(
		PerformSetAvatarURLPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformSetAvatarURL", s.SetAvatarURL),
	)

	internalAPIMux.Handle(
		QueryNumericLocalpartPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryNumericLocalpart", s.QueryNumericLocalpart),
	)

	internalAPIMux.Handle(
		QueryAccountAvailabilityPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountAvailability", s.QueryAccountAvailability),
	)

	internalAPIMux.Handle(
		QueryAccountByPasswordPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryAccountByPassword", s.QueryAccountByPassword),
	)

	internalAPIMux.Handle(
		PerformSetDisplayNamePath,
		httputil.MakeInternalRPCAPI("UserAPISetDisplayName", s.SetDisplayName),
	)

	internalAPIMux.Handle(
		QueryLocalpartForThreePIDPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryLocalpartForThreePID", s.QueryLocalpartForThreePID),
	)

	internalAPIMux.Handle(
		QueryThreePIDsForLocalpartPath,
		httputil.MakeInternalRPCAPI("UserAPIQueryThreePIDsForLocalpart", s.QueryThreePIDsForLocalpart),
	)

	internalAPIMux.Handle(
		PerformForgetThreePIDPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformForgetThreePID", s.PerformForgetThreePID),
	)

	internalAPIMux.Handle(
		PerformSaveThreePIDAssociationPath,
		httputil.MakeInternalRPCAPI("UserAPIPerformSaveThreePIDAssociation", s.PerformSaveThreePIDAssociation),
	)
}
