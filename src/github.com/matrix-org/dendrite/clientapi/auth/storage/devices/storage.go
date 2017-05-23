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

package devices

import (
	"github.com/matrix-org/dendrite/clientapi/auth/authtypes"
)

// Database represents a device database.
type Database struct {
	// TODO
}

// NewDatabase creates a new device database
func NewDatabase(dataSource string) (*Database, error) {
	return &Database{}, nil
}

// GetDeviceByAccessToken returns the device matching the given access token.
func (d *Database) GetDeviceByAccessToken(token string) (*authtypes.Device, error) {
	// TODO: Actual implementation
	return &authtypes.Device{
		UserID: token,
	}, nil
}
