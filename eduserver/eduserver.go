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
	"github.com/matrix-org/dendrite/setup/base"
	"github.com/matrix-org/dendrite/setup/jetstream"
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
	base *base.BaseDendrite,
	eduCache *cache.EDUCache,
	userAPI userapi.UserInternalAPI,
) api.EDUServerInputAPI {
	cfg := &base.Cfg.EDUServer

	js, _ := jetstream.Prepare(base.ProcessContext, &cfg.Matrix.JetStream)

	return &input.EDUServerInputAPI{
		Cache:                        eduCache,
		UserAPI:                      userAPI,
		JetStream:                    js,
		OutputTypingEventTopic:       cfg.Matrix.JetStream.TopicFor(jetstream.OutputTypingEvent),
		OutputSendToDeviceEventTopic: cfg.Matrix.JetStream.TopicFor(jetstream.OutputSendToDeviceEvent),
		OutputReceiptEventTopic:      cfg.Matrix.JetStream.TopicFor(jetstream.OutputReceiptEvent),
		ServerName:                   cfg.Matrix.ServerName,
	}
}
