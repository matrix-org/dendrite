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
	"net/http"

	"github.com/matrix-org/dendrite/common/basecomponent"
	"github.com/matrix-org/dendrite/eduserver/api"
	"github.com/matrix-org/dendrite/eduserver/cache"
	"github.com/matrix-org/dendrite/eduserver/input"
)

// SetupTypingServerComponent sets up and registers HTTP handlers for the
// TypingServer component. Returns instances of the various roomserver APIs,
// allowing other components running in the same process to hit the query the
// APIs directly instead of having to use HTTP.
func SetupTypingServerComponent(
	base *basecomponent.BaseDendrite,
	typingCache *cache.TypingCache,
) api.TypingServerInputAPI {
	inputAPI := &input.TypingServerInputAPI{
		Cache:                  typingCache,
		Producer:               base.KafkaProducer,
		OutputTypingEventTopic: string(base.Cfg.Kafka.Topics.OutputTypingEvent),
	}

	inputAPI.SetupHTTP(http.DefaultServeMux)
	return inputAPI
}
