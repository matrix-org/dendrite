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

package keyserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/keyserver/routing"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// AddPublicRoutes registers HTTP handlers for CS API calls
func AddPublicRoutes(
	router *mux.Router, cfg *config.Dendrite, userAPI userapi.UserInternalAPI,
) {
	routing.Setup(router, cfg, userAPI)
}
