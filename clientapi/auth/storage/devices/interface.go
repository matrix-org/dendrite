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

package devices

import (
	"context"

	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

type Database interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*authtypes.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*authtypes.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]authtypes.Device, error)
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string) (dev *authtypes.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveDevices(ctx context.Context, localpart string, devices []string) error
	RemoveAllDevices(ctx context.Context, localpart string) error
}
