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

package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matrix-org/util"
)

// UserInternalAPITrace wraps a RoomserverInternalAPI and logs the
// complete request/response/error
type UserInternalAPITrace struct {
	Impl UserInternalAPI
}

func (t *UserInternalAPITrace) InputAccountData(ctx context.Context, req *InputAccountDataRequest, res *InputAccountDataResponse) error {
	err := t.Impl.InputAccountData(ctx, req, res)
	util.GetLogger(ctx).Infof("InputAccountData req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformAccountCreation(ctx context.Context, req *PerformAccountCreationRequest, res *PerformAccountCreationResponse) error {
	err := t.Impl.PerformAccountCreation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformAccountCreation req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformPasswordUpdate(ctx context.Context, req *PerformPasswordUpdateRequest, res *PerformPasswordUpdateResponse) error {
	err := t.Impl.PerformPasswordUpdate(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPasswordUpdate req=%+v res=%+v", js(req), js(res))
	return err
}

func (t *UserInternalAPITrace) PerformDeviceCreation(ctx context.Context, req *PerformDeviceCreationRequest, res *PerformDeviceCreationResponse) error {
	err := t.Impl.PerformDeviceCreation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformDeviceCreation req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformDeviceDeletion(ctx context.Context, req *PerformDeviceDeletionRequest, res *PerformDeviceDeletionResponse) error {
	err := t.Impl.PerformDeviceDeletion(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformDeviceDeletion req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformLastSeenUpdate(ctx context.Context, req *PerformLastSeenUpdateRequest, res *PerformLastSeenUpdateResponse) error {
	err := t.Impl.PerformLastSeenUpdate(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformLastSeenUpdate req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformDeviceUpdate(ctx context.Context, req *PerformDeviceUpdateRequest, res *PerformDeviceUpdateResponse) error {
	err := t.Impl.PerformDeviceUpdate(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformDeviceUpdate req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformAccountDeactivation(ctx context.Context, req *PerformAccountDeactivationRequest, res *PerformAccountDeactivationResponse) error {
	err := t.Impl.PerformAccountDeactivation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformAccountDeactivation req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformOpenIDTokenCreation(ctx context.Context, req *PerformOpenIDTokenCreationRequest, res *PerformOpenIDTokenCreationResponse) error {
	err := t.Impl.PerformOpenIDTokenCreation(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformOpenIDTokenCreation req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformKeyBackup(ctx context.Context, req *PerformKeyBackupRequest, res *PerformKeyBackupResponse) error {
	err := t.Impl.PerformKeyBackup(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformKeyBackup req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformPusherSet(ctx context.Context, req *PerformPusherSetRequest, res *struct{}) error {
	err := t.Impl.PerformPusherSet(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPusherSet req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformPusherDeletion(ctx context.Context, req *PerformPusherDeletionRequest, res *struct{}) error {
	err := t.Impl.PerformPusherDeletion(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPusherDeletion req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) PerformPushRulesPut(ctx context.Context, req *PerformPushRulesPutRequest, res *struct{}) error {
	err := t.Impl.PerformPushRulesPut(ctx, req, res)
	util.GetLogger(ctx).Infof("PerformPushRulesPut req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryKeyBackup(ctx context.Context, req *QueryKeyBackupRequest, res *QueryKeyBackupResponse) {
	t.Impl.QueryKeyBackup(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryKeyBackup req=%+v res=%+v", js(req), js(res))
}
func (t *UserInternalAPITrace) QueryProfile(ctx context.Context, req *QueryProfileRequest, res *QueryProfileResponse) error {
	err := t.Impl.QueryProfile(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryProfile req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryAccessToken(ctx context.Context, req *QueryAccessTokenRequest, res *QueryAccessTokenResponse) error {
	err := t.Impl.QueryAccessToken(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryAccessToken req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryDevices(ctx context.Context, req *QueryDevicesRequest, res *QueryDevicesResponse) error {
	err := t.Impl.QueryDevices(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryDevices req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryAccountData(ctx context.Context, req *QueryAccountDataRequest, res *QueryAccountDataResponse) error {
	err := t.Impl.QueryAccountData(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryAccountData req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryDeviceInfos(ctx context.Context, req *QueryDeviceInfosRequest, res *QueryDeviceInfosResponse) error {
	err := t.Impl.QueryDeviceInfos(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryDeviceInfos req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QuerySearchProfiles(ctx context.Context, req *QuerySearchProfilesRequest, res *QuerySearchProfilesResponse) error {
	err := t.Impl.QuerySearchProfiles(ctx, req, res)
	util.GetLogger(ctx).Infof("QuerySearchProfiles req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryOpenIDToken(ctx context.Context, req *QueryOpenIDTokenRequest, res *QueryOpenIDTokenResponse) error {
	err := t.Impl.QueryOpenIDToken(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryOpenIDToken req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryPushers(ctx context.Context, req *QueryPushersRequest, res *QueryPushersResponse) error {
	err := t.Impl.QueryPushers(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryPushers req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryPushRules(ctx context.Context, req *QueryPushRulesRequest, res *QueryPushRulesResponse) error {
	err := t.Impl.QueryPushRules(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryPushRules req=%+v res=%+v", js(req), js(res))
	return err
}
func (t *UserInternalAPITrace) QueryNotifications(ctx context.Context, req *QueryNotificationsRequest, res *QueryNotificationsResponse) error {
	err := t.Impl.QueryNotifications(ctx, req, res)
	util.GetLogger(ctx).Infof("QueryNotifications req=%+v res=%+v", js(req), js(res))
	return err
}

func js(thing interface{}) string {
	b, err := json.Marshal(thing)
	if err != nil {
		return fmt.Sprintf("Marshal error:%s", err)
	}
	return string(b)
}
