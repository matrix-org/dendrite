/* Copyright 2017 New Vector Ltd
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gomatrixserverlib

import (
	"encoding/json"
	"testing"
)

func benchmarkParse(b *testing.B, eventJSON string) {
	var event Event

	// run the Unparse function b.N times
	for n := 0; n < b.N; n++ {
		if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
			b.Error("Failed to parse event")
		}
	}
}

// Benchmark a more complicated event, in this case a power levels event.

func BenchmarkParseLargerEvent(b *testing.B) {
	benchmarkParse(b, `{"auth_events":[["$Stdin0028C5qBjz5:localhost",{"sha256":"PvTyW+Mfb0aCajkIlBk1XlQE+1uVco3to8C2+/1J7iQ"}],["$klXtjBwwDQIGglax:localhost",{"sha256":"hLoiSkcGLZJr5wkIDA8+bujNJPsYX1SOCCXIErHEcgM"}]],"content":{"ban":50,"events":{"m.room.avatar":50,"m.room.canonical_alias":50,"m.room.history_visibility":100,"m.room.name":50,"m.room.power_levels":100},"events_default":0,"invite":0,"kick":50,"redact":50,"state_default":50,"users":{"@test:localhost":100},"users_default":0},"depth":3,"event_id":"$7gPR7SLdkfDsMvJL:localhost","hashes":{"sha256":"/kQnrzO5vhbnwyGvKso4CVMRyyryiyanq6t27mt5kSw"},"origin":"localhost","origin_server_ts":1510854446548,"prev_events":[["$klXtjBwwDQIGglax:localhost",{"sha256":"hLoiSkcGLZJr5wkIDA8+bujNJPsYX1SOCCXIErHEcgM"}]],"prev_state":[],"room_id":"!pUjJbIC8V32G0FLt:localhost","sender":"@test:localhost","signatures":{"localhost":{"ed25519:u9kP":"NOxjrcci7AIRhcTVmJ6nrsslLsaOJzB0iusDZ6cOFrv2OXkDY7mrBM3cQQS3DhGWltEtu3OC0nsvkfeYtwr9DQ"}},"state_key":"","type":"m.room.power_levels"}`)
}

// Lets now test parsing a smaller name event, first one that is valid, then wrong hash, and then the redacted one

func BenchmarkParseSmallerEvent(b *testing.B) {
	benchmarkParse(b, `{"auth_events":[["$oXL79cT7fFxR7dPH:localhost",{"sha256":"abjkiDSg1RkuZrbj2jZoGMlQaaj1Ue3Jhi7I7NlKfXY"}],["$IVUsaSkm1LBAZYYh:localhost",{"sha256":"X7RUj46hM/8sUHNBIFkStbOauPvbDzjSdH4NibYWnko"}],["$VS2QT0EeArZYi8wf:localhost",{"sha256":"k9eM6utkCH8vhLW9/oRsH74jOBS/6RVK42iGDFbylno"}]],"content":{"name":"test3"},"depth":7,"event_id":"$yvN1b43rlmcOs5fY:localhost","hashes":{"sha256":"Oh1mwI1jEqZ3tgJ+V1Dmu5nOEGpCE4RFUqyJv2gQXKs"},"origin":"localhost","origin_server_ts":1510854416361,"prev_events":[["$FqI6TVvWpcbcnJ97:localhost",{"sha256":"upCsBqUhNUgT2/+zkzg8TbqdQpWWKQnZpGJc6KcbUC4"}]],"prev_state":[],"room_id":"!19Mp0U9hjajeIiw1:localhost","sender":"@test:localhost","signatures":{"localhost":{"ed25519:u9kP":"5IzSuRXkxvbTp0vZhhXYZeOe+619iG3AybJXr7zfNn/4vHz4TH7qSJVQXSaHHvcTcDodAKHnTG1WDulgO5okAQ"}},"state_key":"","type":"m.room.name"}`)
}

func BenchmarkParseSmallerEventFailedHash(b *testing.B) {
	benchmarkParse(b, `{"auth_events":[["$oXL79cT7fFxR7dPH:localhost",{"sha256":"abjkiDSg1RkuZrbj2jZoGMlQaaj1Ue3Jhi7I7NlKfXY"}],["$IVUsaSkm1LBAZYYh:localhost",{"sha256":"X7RUj46hM/8sUHNBIFkStbOauPvbDzjSdH4NibYWnko"}],["$VS2QT0EeArZYi8wf:localhost",{"sha256":"k9eM6utkCH8vhLW9/oRsH74jOBS/6RVK42iGDFbylno"}]],"content":{"name":"test4"},"depth":7,"event_id":"$yvN1b43rlmcOs5fY:localhost","hashes":{"sha256":"Oh1mwI1jEqZ3tgJ+V1Dmu5nOEGpCE4RFUqyJv2gQXKs"},"origin":"localhost","origin_server_ts":1510854416361,"prev_events":[["$FqI6TVvWpcbcnJ97:localhost",{"sha256":"upCsBqUhNUgT2/+zkzg8TbqdQpWWKQnZpGJc6KcbUC4"}]],"prev_state":[],"room_id":"!19Mp0U9hjajeIiw1:localhost","sender":"@test:localhost","signatures":{"localhost":{"ed25519:u9kP":"5IzSuRXkxvbTp0vZhhXYZeOe+619iG3AybJXr7zfNn/4vHz4TH7qSJVQXSaHHvcTcDodAKHnTG1WDulgO5okAQ"}},"state_key":"","type":"m.room.name"}`)
}

func BenchmarkParseSmallerEventRedacted(b *testing.B) {
	benchmarkParse(b, `{"event_id":"$yvN1b43rlmcOs5fY:localhost","sender":"@test:localhost","room_id":"!19Mp0U9hjajeIiw1:localhost","hashes":{"sha256":"Oh1mwI1jEqZ3tgJ+V1Dmu5nOEGpCE4RFUqyJv2gQXKs"},"signatures":{"localhost":{"ed25519:u9kP":"5IzSuRXkxvbTp0vZhhXYZeOe+619iG3AybJXr7zfNn/4vHz4TH7qSJVQXSaHHvcTcDodAKHnTG1WDulgO5okAQ"}},"content":{},"type":"m.room.name","state_key":"","depth":7,"prev_events":[["$FqI6TVvWpcbcnJ97:localhost",{"sha256":"upCsBqUhNUgT2/+zkzg8TbqdQpWWKQnZpGJc6KcbUC4"}]],"prev_state":[],"auth_events":[["$oXL79cT7fFxR7dPH:localhost",{"sha256":"abjkiDSg1RkuZrbj2jZoGMlQaaj1Ue3Jhi7I7NlKfXY"}],["$IVUsaSkm1LBAZYYh:localhost",{"sha256":"X7RUj46hM/8sUHNBIFkStbOauPvbDzjSdH4NibYWnko"}],["$VS2QT0EeArZYi8wf:localhost",{"sha256":"k9eM6utkCH8vhLW9/oRsH74jOBS/6RVK42iGDFbylno"}]],"origin":"localhost","origin_server_ts":1510854416361}`)
}
