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

package main

// nolint: varcheck, deadcode
const (
	i0StateRoomCreate = iota
	i1StateAliceJoin
	i2StatePowerLevels
	i3StateJoinRules
	i4StateHistoryVisibility
	i5AliceMsg
	i6AliceMsg
	i7AliceMsg
	i8StateAliceRoomName
	i9StateBobJoin
	i10BobMsg
	i11StateAliceRoomName
	i12AliceMsg
	i13StateBobInviteCharlie
	i14StateCharlieJoin
	i15AliceMsg
	i16StateAliceKickCharlie
	i17BobMsg
	i18StateAliceRoomName
	i19BobMsg
	i20StateBobLeave
	i21AliceMsg
	i22StateAliceInviteBob
	i23StateBobRejectInvite
	i24AliceMsg
	i25StateAliceRoomName
	i26StateCharlieJoin
	i27CharlieMsg
)

var outputRoomEventTestData = []string{
	// $ curl -XPOST -d '{}' "http://localhost:8009/_matrix/client/r0/createRoom?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[],"content":{"creator":"@alice:localhost"},"depth":1,"event_id":"$xz0fUB8zNMTGFh1W:localhost","hashes":{"sha256":"KKkpxS8NoH0igBbL3J+nJ39MRlmA7QgW4BGL7Fv4ASI"},"origin":"localhost","origin_server_ts":1494411218382,"prev_events":[],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"uZG5Q/Hs2Z611gFlZPdwomomRJKf70xV2FQV+gLWM1XgzkLDRlRF3cBZc9y3CnHKnV/upTcXs7Op2/GmgD3UBw"}},"state_key":"","type":"m.room.create"},"latest_event_ids":["$xz0fUB8zNMTGFh1W:localhost"],"adds_state_event_ids":["$xz0fUB8zNMTGFh1W:localhost"],"last_sent_event_id":""}}`,
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}]],"content":{"membership":"join"},"depth":2,"event_id":"$QTen1vksfcRTpUCk:localhost","hashes":{"sha256":"tTukc9ab1fJfzgc5EMA/UD3swqfl/ic9Y9Zkt4fJo0Q"},"origin":"localhost","origin_server_ts":1494411218385,"prev_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"OPysDn/wT7yHeALXLTcEgR+iaKjv0p7VPuR/Mzvyg2IMAwPUjSOw8SQZlhSioWRtVPUp9VHbhIhJxQaPUg9yBQ"}},"state_key":"@alice:localhost","type":"m.room.member"},"latest_event_ids":["$QTen1vksfcRTpUCk:localhost"],"adds_state_event_ids":["$QTen1vksfcRTpUCk:localhost"],"last_sent_event_id":"$xz0fUB8zNMTGFh1W:localhost"}}`,
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}]],"content":{"ban":50,"events":{"m.room.avatar":50,"m.room.canonical_alias":50,"m.room.history_visibility":100,"m.room.name":50,"m.room.power_levels":100},"events_default":0,"invite":0,"kick":50,"redact":50,"state_default":50,"users":{"@alice:localhost":100},"users_default":0},"depth":3,"event_id":"$RWsxGlfPHAcijTgu:localhost","hashes":{"sha256":"ueZWiL/Q8bagRQGFktpnYJAJV6V6U3QKcUEmWYeyaaM"},"origin":"localhost","origin_server_ts":1494411218385,"prev_events":[["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"hZwWx3lyW61zMYmqLOxLTlfW2CnbjJQsZPLjZFa97TVG4ISz8CixMPsnVAIu5is29UCmiHyP8RvLecJjbLCtAQ"}},"state_key":"","type":"m.room.power_levels"},"latest_event_ids":["$RWsxGlfPHAcijTgu:localhost"],"adds_state_event_ids":["$RWsxGlfPHAcijTgu:localhost"],"last_sent_event_id":"$QTen1vksfcRTpUCk:localhost"}}`,
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}]],"content":{"join_rule":"public"},"depth":4,"event_id":"$2O2DpHB37CuwwJOe:localhost","hashes":{"sha256":"3P3HxAXI8gc094i020EoV/gissYiMVWv8+JAbrakM4E"},"origin":"localhost","origin_server_ts":1494411218386,"prev_events":[["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"L2yZoBbG/6TNsRHz+UtHY0SK4FgrdAYPR1l7RBWaNFbm+k/7kVhnoGlJ9yptpdLJjPMR2InqKXH8BBxRC83BCg"}},"state_key":"","type":"m.room.join_rules"},"latest_event_ids":["$2O2DpHB37CuwwJOe:localhost"],"adds_state_event_ids":["$2O2DpHB37CuwwJOe:localhost"],"last_sent_event_id":"$RWsxGlfPHAcijTgu:localhost"}}`,
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}]],"content":{"history_visibility":"joined"},"depth":5,"event_id":"$5LRiBskVCROnL5WY:localhost","hashes":{"sha256":"341alVufcKSVKLPr9WsJNTnW33QkBTn9eTfVWbyoa0o"},"origin":"localhost","origin_server_ts":1494411218387,"prev_events":[["$2O2DpHB37CuwwJOe:localhost",{"sha256":"ulaRD63dbCyolLTwvInIQpcrtU2c7ex/BHmhpLXAUoE"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"kRyt68cstwYgK8NtYzf0V5CnAbqUO47ixCCWYzRCi0WNstEwUw4XW1GHc8BllQsXwSj+nNv9g/66zZgG0DtxCA"}},"state_key":"","type":"m.room.history_visibility"},"latest_event_ids":["$5LRiBskVCROnL5WY:localhost"],"adds_state_event_ids":["$5LRiBskVCROnL5WY:localhost"],"last_sent_event_id":"$2O2DpHB37CuwwJOe:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"hello world","msgtype":"m.text"},"depth":0,"event_id":"$Z8ZJik7ghwzSYTH9:localhost","hashes":{"sha256":"ahN1T5aiSZCzllf0pqNWJkF+x2h2S3kic+40pQ1X6BE"},"origin":"localhost","origin_server_ts":1494411339207,"prev_events":[["$5LRiBskVCROnL5WY:localhost",{"sha256":"3jULNC9b9Q0AhvnDQqpjhbtYwmkioHzPzdTJZvn8vOI"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"ylEpahRwEfGpqk+UCv0IF8YAxmut7w7udgHy3sVDfdJhs/4uJ6EkFEsKLknpXRc1vTIy1etKCBQ63QbCmRC2Bw"}},"type":"m.room.message"},"latest_event_ids":["$Z8ZJik7ghwzSYTH9:localhost"],"last_sent_event_id":"$5LRiBskVCROnL5WY:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 2"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"hello world 2","msgtype":"m.text"},"depth":0,"event_id":"$8382Ah682eL4hxjN:localhost","hashes":{"sha256":"hQElDGSYc6KOdylrbMMm3+LlvUiCKo6S9G9n58/qtns"},"origin":"localhost","origin_server_ts":1494411380282,"prev_events":[["$Z8ZJik7ghwzSYTH9:localhost",{"sha256":"FBDwP+2FeqDENe7AEa3iAFAVKl1/IVq43mCH0uPRn90"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"LFXi6jTG7qn9xzi4rhIiHbkLD+4AZ9Yg7UTS2gqm1gt2lXQsgTYH1wE4Fol2fq4lvGlQVpxhtEr2huAYSbT7DA"}},"type":"m.room.message"},"latest_event_ids":["$8382Ah682eL4hxjN:localhost"],"last_sent_event_id":"$Z8ZJik7ghwzSYTH9:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello world 3"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"hello world 3","msgtype":"m.text"},"depth":0,"event_id":"$17SfHsvSeTQthSWF:localhost","hashes":{"sha256":"eS6VFQI0l2U8rA8U17jgSHr9lQ73SNSnlnZu+HD0IjE"},"origin":"localhost","origin_server_ts":1494411396560,"prev_events":[["$8382Ah682eL4hxjN:localhost",{"sha256":"c6I/PUY7WnvxQ+oUEp/w2HEEuD3g8Vq7QwPUOSUjuc8"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"dvu9bSHZmX+yZoEqHioK7YDMtLH9kol0DdFqc5aHsbhZe/fKRZpfJMrlf1iXQdXSCMhikvnboPAXN3guiZCUBQ"}},"type":"m.room.message"},"latest_event_ids":["$17SfHsvSeTQthSWF:localhost"],"last_sent_event_id":"$8382Ah682eL4hxjN:localhost"}}`,
	// $ curl -XPUT -d '{"name":"Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"name":"Custom Room Name"},"depth":0,"event_id":"$j7KtuOzM0K15h3Kr:localhost","hashes":{"sha256":"QIKj5Klr50ugll4EjaNUATJmrru4CDp6TvGPv0v15bo"},"origin":"localhost","origin_server_ts":1494411482625,"prev_events":[["$17SfHsvSeTQthSWF:localhost",{"sha256":"iMTefewJ4W5sKQy7osQv4ilJAi7X0NsK791kqEUmYX0"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"WU7lwSWUAk7bsyDnBs128PyXxPZZoD1sN4AiDcvk+W1mDezJbFvWHDWymclxWESlP7TDrFTZEumRWGGCakjyAg"}},"state_key":"","type":"m.room.name"},"latest_event_ids":["$j7KtuOzM0K15h3Kr:localhost"],"adds_state_event_ids":["$j7KtuOzM0K15h3Kr:localhost"],"last_sent_event_id":"$17SfHsvSeTQthSWF:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$2O2DpHB37CuwwJOe:localhost",{"sha256":"ulaRD63dbCyolLTwvInIQpcrtU2c7ex/BHmhpLXAUoE"}]],"content":{"membership":"join"},"depth":0,"event_id":"$wPepDhIla765Odre:localhost","hashes":{"sha256":"KeKqWLvM+LTvyFbwx6y3Y4W5Pj6nBSFUQ6jpkSf1oTE"},"origin":"localhost","origin_server_ts":1494411534290,"prev_events":[["$j7KtuOzM0K15h3Kr:localhost",{"sha256":"oDrWG5/sy1Ea3hYDOSJZRuGKCcjaHQlDYPDn2gB0/L0"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"oVtvjZbWFe+iJhoDvLcQKnFpSYQ94dOodM4gGsx26P6fs2sFJissYwSIqpoxlElCJnmBAgy5iv4JK/5x21R2CQ"}},"state_key":"@bob:localhost","type":"m.room.member"},"latest_event_ids":["$wPepDhIla765Odre:localhost"],"adds_state_event_ids":["$wPepDhIla765Odre:localhost"],"last_sent_event_id":"$j7KtuOzM0K15h3Kr:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello alice"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/1?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"content":{"body":"hello alice","msgtype":"m.text"},"depth":0,"event_id":"$RHNjeYUvXVZfb93t:localhost","hashes":{"sha256":"Ic1QLxTWFrWt1o31DS93ftrNHkunf4O6ubFvdD4ydNI"},"origin":"localhost","origin_server_ts":1494411593196,"prev_events":[["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"8BHHkiThWwiIZbXCegRjIKNVGIa2kqrZW8VuL7nASfJBORhZ9R9p34UsmhsxVwTs/2/dX7M2ogMB28gIGdLQCg"}},"type":"m.room.message"},"latest_event_ids":["$RHNjeYUvXVZfb93t:localhost"],"last_sent_event_id":"$wPepDhIla765Odre:localhost"}}`,
	// $ curl -XPUT -d '{"name":"A Different Custom Room Name"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"name":"A Different Custom Room Name"},"depth":0,"event_id":"$1xoUuqOFjFFJgwA5:localhost","hashes":{"sha256":"2pNnLhoHxNeSUpqxrd3c0kZUA4I+cdWZgYcJ8V3e2tk"},"origin":"localhost","origin_server_ts":1494411643348,"prev_events":[["$RHNjeYUvXVZfb93t:localhost",{"sha256":"LqFmTIzULgUDSf5xM3REObvnsRGLQliWBUf1hEDT4+w"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"gsY4B6TIBdVvLyFAaXw0xez9N5/Cn/ZaJ4z+j9gJU/ZR8j1t3OYlcVQN6uln9JwEU1k20AsGnIqvOaayd+bfCg"}},"state_key":"","type":"m.room.name"},"latest_event_ids":["$1xoUuqOFjFFJgwA5:localhost"],"adds_state_event_ids":["$1xoUuqOFjFFJgwA5:localhost"],"removes_state_event_ids":["$j7KtuOzM0K15h3Kr:localhost"],"last_sent_event_id":"$RHNjeYUvXVZfb93t:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hello bob"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/2?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"hello bob","msgtype":"m.text"},"depth":0,"event_id":"$4NBTdIwDxq5fDGpv:localhost","hashes":{"sha256":"msCIESAya8kD7nLCopxkEqrgVuGfrlr9YBIADH5czTA"},"origin":"localhost","origin_server_ts":1494411674630,"prev_events":[["$1xoUuqOFjFFJgwA5:localhost",{"sha256":"ZXj+kY6sqQpf5vsNqvCMSvNoXXKDKxRE4R7+gZD9Tkk"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"bZRT3NxVlfBWw1PxSlKlgfnJixG+NI5H9QmUK2AjECg+l887BZJNCvAK0eD27N8e9V+c2glyXWYje2wexP2CBw"}},"type":"m.room.message"},"latest_event_ids":["$4NBTdIwDxq5fDGpv:localhost"],"last_sent_event_id":"$1xoUuqOFjFFJgwA5:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"invite"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"content":{"membership":"invite"},"depth":0,"event_id":"$zzLHVlHIWPrnE7DI:localhost","hashes":{"sha256":"LKk7tnYJAHsyffbi9CzfdP+TU4KQ5g6YTgYGKjJ7NxU"},"origin":"localhost","origin_server_ts":1494411709192,"prev_events":[["$4NBTdIwDxq5fDGpv:localhost",{"sha256":"EpqmxEoJP93Zb2Nt2fS95SJWTqqIutHm/Ne8OHqp6Ps"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"GdUzkC+7YKl1XDi7kYuD39yi2L/+nv+YrecIQHS+0BLDQqnEj+iRXfNBuZfTk6lUBCJCHXZlk7MnEIjvWDlZCg"}},"state_key":"@charlie:localhost","type":"m.room.member"},"latest_event_ids":["$zzLHVlHIWPrnE7DI:localhost"],"adds_state_event_ids":["$zzLHVlHIWPrnE7DI:localhost"],"last_sent_event_id":"$4NBTdIwDxq5fDGpv:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@charlie:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$2O2DpHB37CuwwJOe:localhost",{"sha256":"ulaRD63dbCyolLTwvInIQpcrtU2c7ex/BHmhpLXAUoE"}],["$zzLHVlHIWPrnE7DI:localhost",{"sha256":"Jw28x9W+GoZYw7sEynsi1fcRzqRQiLddolOa/p26PV0"}]],"content":{"membership":"join"},"unsigned":{"prev_content":{"membership":"invite"},"prev_sender":"@bob:localhost","replaces_state":"$zzLHVlHIWPrnE7DI:localhost"},"depth":0,"event_id":"$uJVKyzZi8ZX0kOd9:localhost","hashes":{"sha256":"9ZZs/Cg0ewpBiCB6iFXXYlmW8koFiesCNGFrOLDTolE"},"origin":"localhost","origin_server_ts":1494411745015,"prev_events":[["$zzLHVlHIWPrnE7DI:localhost",{"sha256":"Jw28x9W+GoZYw7sEynsi1fcRzqRQiLddolOa/p26PV0"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@charlie:localhost","signatures":{"localhost":{"ed25519:something":"+TM0gFPM/M3Ji2BjYuTUTgDyCOWlOq8aTMCxLg7EBvS62yPxJ558f13OWWTczUO5aRAt+PvXsMVM/bp8u6c8DQ"}},"state_key":"@charlie:localhost","type":"m.room.member"},"latest_event_ids":["$uJVKyzZi8ZX0kOd9:localhost"],"adds_state_event_ids":["$uJVKyzZi8ZX0kOd9:localhost"],"removes_state_event_ids":["$zzLHVlHIWPrnE7DI:localhost"],"last_sent_event_id":"$zzLHVlHIWPrnE7DI:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"not charlie..."}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"not charlie...","msgtype":"m.text"},"depth":0,"event_id":"$Ixfn5WT9ocWTYxfy:localhost","hashes":{"sha256":"hRChdyMQ3AY4jvrPpI8PEX6Taux83Qo5hdSeHlhPxGo"},"origin":"localhost","origin_server_ts":1494411792737,"prev_events":[["$uJVKyzZi8ZX0kOd9:localhost",{"sha256":"BtesLFnHZOREQCeilFM+xvDU/Wdj+nyHMw7IGTh/9gU"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"LC/Zqwu/XdqjmLdTOp/NQaFaE0niSAGgEpa39gCxsnsqEX80P7P5WDn/Kzx6rjWTnhIszrLsnoycqkXQT0Z4DQ"}},"type":"m.room.message"},"latest_event_ids":["$Ixfn5WT9ocWTYxfy:localhost"],"last_sent_event_id":"$uJVKyzZi8ZX0kOd9:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$uJVKyzZi8ZX0kOd9:localhost",{"sha256":"BtesLFnHZOREQCeilFM+xvDU/Wdj+nyHMw7IGTh/9gU"}]],"content":{"membership":"leave"},"unsigned":{"prev_content":{"membership":"join"},"prev_sender":"@charlie:localhost","replaces_state":"$uJVKyzZi8ZX0kOd9:localhost"},"depth":0,"event_id":"$om1F4AI8tCYlHUSp:localhost","hashes":{"sha256":"7JVI0uCxSUyEqDJ+o36/zUIlIZkXVK/R6wkrZGvQXDE"},"origin":"localhost","origin_server_ts":1494411855278,"prev_events":[["$Ixfn5WT9ocWTYxfy:localhost",{"sha256":"hOoPIDQFvvNqQJzA5ggjoQi4v1BOELnhnmwU4UArDOY"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"3sxoDLUPnKuDJgFgS3C647BbiXrozxhhxrZOlFP3KgJKzBYv/ht+Jd2V2iSZOvsv94wgRBf0A/lEcJRIqeLgDA"}},"state_key":"@charlie:localhost","type":"m.room.member"},"latest_event_ids":["$om1F4AI8tCYlHUSp:localhost"],"adds_state_event_ids":["$om1F4AI8tCYlHUSp:localhost"],"removes_state_event_ids":["$uJVKyzZi8ZX0kOd9:localhost"],"last_sent_event_id":"$Ixfn5WT9ocWTYxfy:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"why did you kick charlie"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"content":{"body":"why did you kick charlie","msgtype":"m.text"},"depth":0,"event_id":"$hgao5gTmr3r9TtK2:localhost","hashes":{"sha256":"Aa2ZCrvwjX5xhvkVqIOFUeEGqrnrQZjjNFiZRybjsPY"},"origin":"localhost","origin_server_ts":1494411912809,"prev_events":[["$om1F4AI8tCYlHUSp:localhost",{"sha256":"yVs+CW7AiJrJOYouL8xPIBrtIHAhnbxaegna8MxeCto"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"sGkpbEXGsvAuCvE3wb5E9H5fjCVKpRdWNt6csj1bCB9Fmg4Rg4mvj3TAJ+91DjO8IPsgSxDKdqqRYF0OtcynBA"}},"type":"m.room.message"},"latest_event_ids":["$hgao5gTmr3r9TtK2:localhost"],"last_sent_event_id":"$om1F4AI8tCYlHUSp:localhost"}}`,
	// $ curl -XPUT -d '{"name":"No Charlies"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"name":"No Charlies"},"depth":0,"event_id":"$CY4XDoxjbns3a4Pc:localhost","hashes":{"sha256":"chk72pVkp3AGR2FtdC0mORBWS1b9ePnRN4WK3BP0BiI"},"origin":"localhost","origin_server_ts":1494411959114,"prev_events":[["$hgao5gTmr3r9TtK2:localhost",{"sha256":"/4/OG4Q2YalIeBtN76BEPIieBKA/3UFshR9T+WJip4o"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"mapvA3KJYgw5FmzJMhSFa/+JSuNyv2eKAkiGomAeBB7LQ1e9nK9XhW/Fp7a5Z2Sy2ENwHyd3ij7FEGiLOnSIAw"}},"state_key":"","type":"m.room.name"},"latest_event_ids":["$CY4XDoxjbns3a4Pc:localhost"],"adds_state_event_ids":["$CY4XDoxjbns3a4Pc:localhost"],"removes_state_event_ids":["$1xoUuqOFjFFJgwA5:localhost"],"last_sent_event_id":"$hgao5gTmr3r9TtK2:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"whatever"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"content":{"body":"whatever","msgtype":"m.text"},"depth":0,"event_id":"$pl8VBHRPYDmsnDh4:localhost","hashes":{"sha256":"FYqY9+/cepwIxxjfFV3AjOFBXkTlyEI2jep87dUc+SU"},"origin":"localhost","origin_server_ts":1494411988548,"prev_events":[["$CY4XDoxjbns3a4Pc:localhost",{"sha256":"hCoV63fp8eiquVdEefsOqJtLmJhw4wTlRv+wNTS20Ac"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"sQKwRzE59eZyb8rDySo/pVwZXBh0nA5zx+kjEyXglxIQrTre+8Gj3R7Prni+RE3Dq7oWfKYV7QklTLURAaSICQ"}},"type":"m.room.message"},"latest_event_ids":["$pl8VBHRPYDmsnDh4:localhost"],"last_sent_event_id":"$CY4XDoxjbns3a4Pc:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$wPepDhIla765Odre:localhost",{"sha256":"GqUhRiAkRvPrNBDyUxj+emRfK2P8j6iWtvsXDOUltiI"}]],"content":{"membership":"leave"},"depth":0,"event_id":"$acCW4IgnBo8YD3jw:localhost","hashes":{"sha256":"porP+E2yftBGjfS381+WpZeDM9gZHsM3UydlBcRKBLw"},"origin":"localhost","origin_server_ts":1494412037042,"prev_events":[["$pl8VBHRPYDmsnDh4:localhost",{"sha256":"b+qQ380JDFq7quVU9EbIJ2sbpUKM1LAUNX0ZZUoVMZw"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"kxbjTIC0/UR4cOYUAOTNiUc0SSVIF4BY6Rq6IEgYJemq4jcU2fYqum4mFxIQTDKKXMSRHEoNPDmYMFIJwkrsCg"}},"state_key":"@bob:localhost","type":"m.room.member"},"latest_event_ids":["$acCW4IgnBo8YD3jw:localhost"],"adds_state_event_ids":["$acCW4IgnBo8YD3jw:localhost"],"removes_state_event_ids":["$wPepDhIla765Odre:localhost"],"last_sent_event_id":"$pl8VBHRPYDmsnDh4:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"im alone now"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"im alone now","msgtype":"m.text"},"depth":0,"event_id":"$nYdEXrvTDeb7DfkC:localhost","hashes":{"sha256":"qibC5NmlJpSRMBWSWxy1pv73FXymhPDXQFMmGosfsV0"},"origin":"localhost","origin_server_ts":1494412084668,"prev_events":[["$acCW4IgnBo8YD3jw:localhost",{"sha256":"8h3uXoE6pnI9iLnXI6493qJ0HeuRQfenRIu9PcgH72g"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"EHRoZznhXywhYeIn83o4FSFm3No/aOdLQPHQ68YGtNgESWwpuWLkkGVjoISjz3QgXQ06Fl3cHt7nlTaAHpCNAg"}},"type":"m.room.message"},"latest_event_ids":["$nYdEXrvTDeb7DfkC:localhost"],"last_sent_event_id":"$acCW4IgnBo8YD3jw:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"invite"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$acCW4IgnBo8YD3jw:localhost",{"sha256":"8h3uXoE6pnI9iLnXI6493qJ0HeuRQfenRIu9PcgH72g"}]],"content":{"membership":"invite"},"depth":0,"event_id":"$gKNfcXLlWvs2cFad:localhost","hashes":{"sha256":"iYDOUjYkaGSFbVp7TRVFvGJyGMEuBHMQrJ9XqwhzmPI"},"origin":"localhost","origin_server_ts":1494412135845,"prev_events":[["$nYdEXrvTDeb7DfkC:localhost",{"sha256":"83T5Q3+nDvtS0oJTEhHxIw02twBDa1A7QR2bHtnxv1Y"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"ofw009aMJMqVjww9eDXgeTjOQqSlJl/GN/AAb+6mZAPcUI8aVgRlXOSESfhu1ONEuV/yNUycxNXWfMwuvoWsDg"}},"state_key":"@bob:localhost","type":"m.room.member"},"latest_event_ids":["$gKNfcXLlWvs2cFad:localhost"],"adds_state_event_ids":["$gKNfcXLlWvs2cFad:localhost"],"removes_state_event_ids":["$acCW4IgnBo8YD3jw:localhost"],"last_sent_event_id":"$nYdEXrvTDeb7DfkC:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"leave"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@bob:localhost?access_token=@bob:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$gKNfcXLlWvs2cFad:localhost",{"sha256":"/TYIY+L9qjg516Bzl8sadu+Np21KkxE4KdPXALeJ9eE"}]],"content":{"membership":"leave"},"depth":0,"event_id":"$B2q9Tepb6Xc1Rku0:localhost","hashes":{"sha256":"RbHTVdceAEfTALQDZdGrOmakKeTYnChaKjlVuoNUdSY"},"origin":"localhost","origin_server_ts":1494412187614,"prev_events":[["$gKNfcXLlWvs2cFad:localhost",{"sha256":"/TYIY+L9qjg516Bzl8sadu+Np21KkxE4KdPXALeJ9eE"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@bob:localhost","signatures":{"localhost":{"ed25519:something":"dNtUL86j2zUe5+DkfOkil5VujvFZg4FeTjbtcpeF+3E4SUChCAG3lyR6YOAIYBnjtD0/kqT7OcP3pM6vMEp1Aw"}},"state_key":"@bob:localhost","type":"m.room.member"},"latest_event_ids":["$B2q9Tepb6Xc1Rku0:localhost"],"adds_state_event_ids":["$B2q9Tepb6Xc1Rku0:localhost"],"removes_state_event_ids":["$gKNfcXLlWvs2cFad:localhost"],"last_sent_event_id":"$gKNfcXLlWvs2cFad:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"so alone"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"body":"so alone","msgtype":"m.text"},"depth":0,"event_id":"$W1nrYHQIbCTTSJOV:localhost","hashes":{"sha256":"uUKSa4U1coDoT3LUcNF25dt+UpUa2pLXzRJ3ljgxXZs"},"origin":"localhost","origin_server_ts":1494412229742,"prev_events":[["$B2q9Tepb6Xc1Rku0:localhost",{"sha256":"0CLru7nGPgyF9AWlZnarCElscSVrXl2MMY2atrz80Uc"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"YlBJyDnE34UhaCB9hirQN5OySfTDoqiBDnNvxomXjU94z4a8g2CLWKjApwd/q/j4HamCUtjgkjJ2um6hNjsVBA"}},"type":"m.room.message"},"latest_event_ids":["$W1nrYHQIbCTTSJOV:localhost"],"last_sent_event_id":"$B2q9Tepb6Xc1Rku0:localhost"}}`,
	// $ curl -XPUT -d '{"name":"Everyone welcome"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.name?access_token=@alice:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$QTen1vksfcRTpUCk:localhost",{"sha256":"znwhbYzdueh0grYkUX4jgXmP9AjKphzyesMZWMiF4IY"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}]],"content":{"name":"Everyone welcome"},"depth":0,"event_id":"$nLzxoBC4A0QRvJ1k:localhost","hashes":{"sha256":"PExCybjaMW1TfgFr57MdIRYJ642FY2jnrdW/tpPOf1Y"},"origin":"localhost","origin_server_ts":1494412294551,"prev_events":[["$W1nrYHQIbCTTSJOV:localhost",{"sha256":"HXk/ACcsiaZ/z1f2aZSIhJF8Ih3BWeh1vp+cV/fwoE0"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@alice:localhost","signatures":{"localhost":{"ed25519:something":"RK09L8sQv78y69PNbOLaX8asq5kp51mbqUuct5gd7ZNmaHKnVds6ew06QEn+gHSDAxqQo2tpcfoajp+yMj1HBw"}},"state_key":"","type":"m.room.name"},"latest_event_ids":["$nLzxoBC4A0QRvJ1k:localhost"],"adds_state_event_ids":["$nLzxoBC4A0QRvJ1k:localhost"],"removes_state_event_ids":["$CY4XDoxjbns3a4Pc:localhost"],"last_sent_event_id":"$W1nrYHQIbCTTSJOV:localhost"}}`,
	// $ curl -XPUT -d '{"membership":"join"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/state/m.room.member/@charlie:localhost?access_token=@charlie:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$2O2DpHB37CuwwJOe:localhost",{"sha256":"ulaRD63dbCyolLTwvInIQpcrtU2c7ex/BHmhpLXAUoE"}],["$om1F4AI8tCYlHUSp:localhost",{"sha256":"yVs+CW7AiJrJOYouL8xPIBrtIHAhnbxaegna8MxeCto"}]],"content":{"membership":"join"},"depth":0,"event_id":"$Zo6P8r9bczF6kctV:localhost","hashes":{"sha256":"R3J2iUWnGxVdmly8ah+Dgb5VbJ2i/e8BLaWM0z9eZKU"},"origin":"localhost","origin_server_ts":1494412338689,"prev_events":[["$nLzxoBC4A0QRvJ1k:localhost",{"sha256":"TDcFaArAXpxIJ1noSubcFqkLXiQTrc1Dw1+kgCtx3XY"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@charlie:localhost","signatures":{"localhost":{"ed25519:something":"tVnjLVoJ9SLlMQIJSK/6zANWaEu8tVVkx3AEJiC3y5JmhPORb3PyG8eE+e/9hC4aJSQL8LGLaJNWXukMpb2SBg"}},"state_key":"@charlie:localhost","type":"m.room.member"},"latest_event_ids":["$Zo6P8r9bczF6kctV:localhost"],"adds_state_event_ids":["$Zo6P8r9bczF6kctV:localhost"],"removes_state_event_ids":["$om1F4AI8tCYlHUSp:localhost"],"last_sent_event_id":"$nLzxoBC4A0QRvJ1k:localhost"}}`,
	// $ curl -XPUT -d '{"msgtype":"m.text","body":"hiiiii"}' "http://localhost:8009/_matrix/client/r0/rooms/%21PjrbIMW2cIiaYF4t:localhost/send/m.room.message/3?access_token=@charlie:localhost"
	`{"type":"new_room_event","new_room_event":{"event":{"auth_events":[["$xz0fUB8zNMTGFh1W:localhost",{"sha256":"F4tTLtltC6f2XKeXq4ZKpMZ5EpditaW+RYQSnYzq3lI"}],["$RWsxGlfPHAcijTgu:localhost",{"sha256":"1zc+86U9vLK1BvTJbeLuYpw9dZqvX2fr8rc3pOF69f8"}],["$Zo6P8r9bczF6kctV:localhost",{"sha256":"mnjt3WTYqwtuyl2Fca+0cgm6moHaNL+W9BqRJTQzdEY"}]],"content":{"body":"hiiiii","msgtype":"m.text"},"depth":0,"event_id":"$YAEvK8u2zkTsjf5P:localhost","hashes":{"sha256":"6hKy61h1tuHjYdfpq2MnaPtGEBAZOUz8FLTtxLwjK5A"},"origin":"localhost","origin_server_ts":1494412375465,"prev_events":[["$Zo6P8r9bczF6kctV:localhost",{"sha256":"mnjt3WTYqwtuyl2Fca+0cgm6moHaNL+W9BqRJTQzdEY"}]],"room_id":"!PjrbIMW2cIiaYF4t:localhost","sender":"@charlie:localhost","signatures":{"localhost":{"ed25519:something":"BsSLaMM5U/YkyvBZ00J/+si9My+wAJZOcBhBeato0oHayiag7FW77ZpSTfADazPdNH62kjB0sdP9CN6vQA7yDg"}},"type":"m.room.message"},"latest_event_ids":["$YAEvK8u2zkTsjf5P:localhost"],"last_sent_event_id":"$Zo6P8r9bczF6kctV:localhost"}}`,
}
