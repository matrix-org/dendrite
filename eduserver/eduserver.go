// Copyright 2017 Vector Creations Ltd
// Copyright 2017-2018 New Vector Ltd
// Copyright 2019-2020 The Matrix.org Foundation C.I.C.
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

package eduserver

import (
	"github.com/gorilla/mux"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/eduserver/input"
	"github.com/matrix-org/dendrite/eduserver/inthttp"
	"github.com/matrix-org/dendrite/internal/config"
	"github.com/matrix-org/dendrite/internal/setup"
	userapi "github.com/matrix-org/dendrite/userapi/api"
)

// AddInternalRoutes registers HTTP handlers for the internal API. Invokes functions
// on the given input API.
func AddInternalRoutes(internalMux *mux.Router, inputAPI api.EDUServerInputAPI) {
	inthttp.AddRoutes(inputAPI, internalMux)
}

// NewInternalAPI returns a concerete implementation of the internal API. Callers
// can call functions directly on the returned API or via an HTTP interface using AddInternalRoutes.
func NewInternalAPI(
	base *setup.BaseDendrite,
	eduCache *cache.EDUCache,
	userAPI userapi.UserInternalAPI,
) api.EDUServerInputAPI {
	cfg := &base.Cfg.EDUServer
	return &input.EDUServerInputAPI{
		Cache:                        eduCache,
		UserAPI:                      userAPI,
		Producer:                     base.KafkaProducer,
		OutputTypingEventTopic:       string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputTypingEvent)),
		OutputSendToDeviceEventTopic: string(cfg.Matrix.Kafka.TopicFor(config.TopicOutputSendToDeviceEvent)),
		ServerName:                   cfg.Matrix.ServerName,
	}
}
