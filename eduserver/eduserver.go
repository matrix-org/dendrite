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
	"github.com/matrix-org/dendrite/clientapi/auth/storage/devices"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/eduserver/input"
	"github.com/matrix-org/dendrite/eduserver/inthttp"
	"github.com/matrix-org/dendrite/internal/basecomponent"
)

// SetupEDUServerComponent sets up and registers HTTP handlers for the
// EDUServer component. Returns instances of the various roomserver APIs,
// allowing other components running in the same process to hit the query the
// APIs directly instead of having to use HTTP.
func SetupEDUServerComponent(
	base *basecomponent.BaseDendrite,
	eduCache *cache.EDUCache,
	deviceDB devices.Database,
) api.EDUServerInputAPI {
	inputAPI := &input.EDUServerInputAPI{
		Cache:                        eduCache,
		DeviceDB:                     deviceDB,
		Producer:                     base.KafkaProducer,
		OutputTypingEventTopic:       string(base.Cfg.Kafka.Topics.OutputTypingEvent),
		OutputSendToDeviceEventTopic: string(base.Cfg.Kafka.Topics.OutputSendToDeviceEvent),
		ServerName:                   base.Cfg.Matrix.ServerName,
	}

	inthttp.AddRoutes(inputAPI, base.InternalAPIMux)

	return inputAPI
}
