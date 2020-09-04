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

	"github.com/matrix-org/dendrite/userapi/api"
)

type Database interface {
	GetDeviceByAccessToken(ctx context.Context, token string) (*api.Device, error)
	GetDeviceByID(ctx context.Context, localpart, deviceID string) (*api.Device, error)
	GetDevicesByLocalpart(ctx context.Context, localpart string) ([]api.Device, error)
	GetDevicesByID(ctx context.Context, deviceIDs []string) ([]api.Device, error)
	// CreateDevice makes a new device associated with the given user ID localpart.
	// If there is already a device with the same device ID for this user, that access token will be revoked
	// and replaced with the given accessToken. If the given accessToken is already in use for another device,
	// an error will be returned.
	// If no device ID is given one is generated.
	// Returns the device on success.
	CreateDevice(ctx context.Context, localpart string, deviceID *string, accessToken string, displayName *string) (dev *api.Device, returnErr error)
	UpdateDevice(ctx context.Context, localpart, deviceID string, displayName *string) error
	RemoveDevice(ctx context.Context, deviceID, localpart string) error
	RemoveDevices(ctx context.Context, localpart string, devices []string) error
	// RemoveAllDevices deleted all devices for this user. Returns the devices deleted.
	RemoveAllDevices(ctx context.Context, localpart, exceptDeviceID string) (devices []api.Device, err error)
}
