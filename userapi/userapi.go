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

package userapi

import (
	"time"

	"github.com/gorilla/mux"
	keyapi "github.com/matrix-org/dendrite/keyserver/api"
	"github.com/matrix-org/dendrite/setup/config"
	"github.com/matrix-org/dendrite/userapi/api"
	"github.com/matrix-org/dendrite/userapi/internal"
	"github.com/matrix-org/dendrite/userapi/inthttp"
	"github.com/matrix-org/dendrite/userapi/storage"
	"github.com/sirupsen/logrus"
)

// defaultLoginTokenLifetime determines how old a valid token may be.
//
// NOTSPEC: The current spec says "SHOULD be limited to around five
// seconds". Since TCP retries are on the order of 3 s, 5 s sounds very low.
// Synapse uses 2 min (https://github.com/matrix-org/synapse/blob/78d5f91de1a9baf4dbb0a794cb49a799f29f7a38/synapse/handlers/auth.py#L1323-L1325).
const defaultLoginTokenLifetime = 2 * time.Minute

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(router *mux.Router, intAPI api.UserInternalAPI) {
	inthttp.AddRoutes(router, intAPI)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	accountDB storage.Database, cfg *config.UserAPI, appServices []config.ApplicationService, keyAPI keyapi.KeyInternalAPI,
) api.UserInternalAPI {
	db, err := storage.NewDatabase(&cfg.DeviceDatabase, cfg.Matrix.ServerName, cfg.BCryptCost, defaultLoginTokenLifetime)
	if err != nil {
		logrus.WithError(err).Panicf("failed to connect to device db")
	}

	return newInternalAPI(db, cfg, appServices, keyAPI)
}

func newInternalAPI(
	db storage.Database,
	cfg *config.UserAPI,
	appServices []config.ApplicationService,
	keyAPI keyapi.KeyInternalAPI,
) api.UserInternalAPI {
	return &internal.UserInternalAPI{
		DB:          db,
		ServerName:  cfg.Matrix.ServerName,
		AppServices: appServices,
		KeyAPI:      keyAPI,
	}
}
